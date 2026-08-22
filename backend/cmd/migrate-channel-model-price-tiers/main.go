package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"

	"gorm.io/gorm"
)

// 该迁移把上一轮“每个分辨率一个渠道模型”的视频 SKU 收敛为一个渠道模型的多个价格档。
// 旧渠道模型、前台 revision、任务、路由尝试与账单订单均保留，避免改变历史恢复和审计语义。
var familyCodes = []string{
	"seedance-2-5", "seedance-2-0", "seedance-2-0-mini", "seedance-2-0-fast",
	"grok-video-1-5", "artdance-2-0", "artdance-2-5", "artdance-2-mini", "artdance-fast",
}

type familyPlan struct {
	logicalModel          model.LogicalModel
	channelModel          model.ChannelModel
	priceTiers            []model.ChannelModelPriceTier
	legacyLogicalModelIDs []string
	legacyChannelModelIDs []string
	alreadySynced         bool
	creatingLogical       bool
}

func main() {
	apply := flag.Bool("apply", false, "创建系统模型价格档并切换当前前台目录版本")
	flag.Parse()
	db, err := database.Open(database.Config{Driver: "postgres", DSN: os.Getenv("DATABASE_URL"), DataDir: os.Getenv("CANVAS_BACKEND_DATA_DIR")})
	if err != nil {
		log.Fatal(err)
	}
	if db.Dialector.Name() != "postgres" {
		log.Fatal("价格档迁移只允许连接 PostgreSQL")
	}
	if err := database.ConfigurePool(db); err != nil {
		log.Fatal(err)
	}
	if err := database.MigrateSchema(db); err != nil {
		log.Fatal(err)
	}
	repo := repository.New(db)
	plans, err := buildPlans(repo)
	if err != nil {
		log.Fatal(err)
	}
	for _, plan := range plans {
		tiers := make([]string, 0, len(plan.priceTiers))
		for _, tier := range plan.priceTiers {
			tiers = append(tiers, fmt.Sprintf("%s=>%s", tier.SelectorKey, tier.ProviderModelKey))
		}
		log.Printf("%s: 系统模型=%s 价格档=[%s] 已同步=%t，待停用旧前台=%d，旧渠道模型=%d", plan.logicalModel.Code, plan.channelModel.ModelKey, strings.Join(tiers, ", "), plan.alreadySynced, len(plan.legacyLogicalModelIDs), len(plan.legacyChannelModelIDs))
	}
	if !*apply {
		log.Print("dry-run 完成；确认后使用 --apply 写入。旧 SKU 与全部历史快照不会删除或改写")
		return
	}
	actor, err := migrationAdmin(db)
	if err != nil {
		log.Fatal(err)
	}
	svc := service.New(repo, os.Getenv("CANVAS_BACKEND_DATA_DIR"))
	for _, plan := range plans {
		if !plan.alreadySynced {
			if err := repo.SaveChannelModelWithPriceTiers(&plan.channelModel, plan.priceTiers); err != nil {
				log.Fatalf("保存 %s 价格档失败：%v", plan.logicalModel.Code, err)
			}
		}
		logicalModelID := plan.logicalModel.ID
		if plan.creatingLogical {
			logicalModelID = ""
		}
		if _, err := svc.SaveAdminLogicalModel(actor, logicalModelID, service.LogicalModelRequest{
			Code: plan.logicalModel.Code, Name: plan.logicalModel.Name, Icon: plan.logicalModel.Icon, Description: plan.logicalModel.Description,
			Capability: plan.channelModel.Capability, Enabled: plan.logicalModel.Enabled, SortOrder: plan.logicalModel.SortOrder,
			LegacyModelIDs: append(decodeLegacyModelIDs(plan.logicalModel.LegacyModelIDsJSON), plan.legacyLogicalModelIDs...), SourceChannelModelID: plan.channelModel.ID,
		}); err != nil {
			log.Fatalf("同步前台模型 %s 失败：%v", plan.logicalModel.Code, err)
		}
		if err := disableLegacySKURecords(db, plan); err != nil {
			log.Fatalf("停用 %s 的旧 SKU 失败：%v", plan.logicalModel.Code, err)
		}
		log.Printf("已同步 %s 到系统模型 %s", plan.logicalModel.Code, plan.channelModel.ModelKey)
	}
	log.Print("迁移完成：新任务将按系统模型规格价格档结算；旧 SKU 与历史任务/账务快照保持不变")
}

func buildPlans(repo *repository.Repository) ([]familyPlan, error) {
	plans := make([]familyPlan, 0, len(familyCodes))
	for _, code := range familyCodes {
		items, err := repo.LogicalModels(true)
		if err != nil {
			return nil, err
		}
		var logicalModel *model.LogicalModel
		for index := range items {
			if items[index].Code == code {
				logicalModel = &items[index]
				break
			}
		}
		if logicalModel == nil {
			return nil, fmt.Errorf("缺少前台模型家族 %s", code)
		}
		graph, err := repo.LogicalModelGraph(logicalModel.ID, true)
		if err != nil {
			return nil, fmt.Errorf("读取前台模型 %s：%w", code, err)
		}
		if graph.Revision == nil || len(graph.Routes) == 0 || len(graph.ChannelModels) == 0 {
			return nil, fmt.Errorf("前台模型 %s 没有当前供应线路", code)
		}
		if logicalModel.SourceChannelModelID != "" {
			source, sourceErr := repo.ChannelModel(logicalModel.SourceChannelModelID)
			if sourceErr != nil {
				return nil, fmt.Errorf("读取已同步模型 %s 的系统源：%w", code, sourceErr)
			}
			plans = append(plans, familyPlan{logicalModel: *logicalModel, channelModel: *source, priceTiers: source.PriceTiers, alreadySynced: true})
			continue
		}
		plan, planErr := buildFamilyPlan(repo, *logicalModel, graph)
		if planErr != nil {
			return nil, planErr
		}
		plans = append(plans, plan)
	}
	imagePlan, imageErr := buildImage2Plan(repo)
	if imageErr != nil {
		return nil, imageErr
	}
	if imagePlan != nil {
		plans = append(plans, *imagePlan)
	}
	return plans, nil
}

// buildImage2Plan merges the three historical 1K/2K/4K records into one system
// model. The exact upstream key remains tier-local so existing channel behaviour
// is retained while the selected quality becomes a request parameter.
func buildImage2Plan(repo *repository.Repository) (*familyPlan, error) {
	all, err := repo.LogicalModels(true)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]model.LogicalModel, len(all))
	for _, item := range all {
		byCode[item.Code] = item
	}
	if existing, found := byCode["image-2"]; found && existing.SourceChannelModelID != "" {
		source, sourceErr := repo.ChannelModel(existing.SourceChannelModelID)
		if sourceErr != nil {
			return nil, sourceErr
		}
		return &familyPlan{logicalModel: existing, channelModel: *source, priceTiers: source.PriceTiers, alreadySynced: true}, nil
	}
	legacyCodes := []string{"image-2-1k", "image-2-2k", "image-2-4k"}
	legacyModels := make([]model.LogicalModel, 0, len(legacyCodes))
	legacyChannelModels := make([]model.ChannelModel, 0, len(legacyCodes))
	for _, code := range legacyCodes {
		logicalModel, found := byCode[code]
		if !found {
			return nil, fmt.Errorf("缺少 GPT Image 2 旧前台模型 %s", code)
		}
		graph, graphErr := repo.LogicalModelGraph(logicalModel.ID, true)
		if graphErr != nil || graph.Revision == nil || len(graph.Routes) != 1 || len(graph.ChannelModels) != 1 {
			return nil, fmt.Errorf("读取 GPT Image 2 旧模型 %s 的供应线路失败", code)
		}
		legacyModels = append(legacyModels, logicalModel)
		legacyChannelModels = append(legacyChannelModels, graph.ChannelModels[0])
	}
	channelID := legacyChannelModels[0].ChannelID
	for _, item := range legacyChannelModels {
		if item.ChannelID != channelID || item.Capability != "image" || item.Protocol != legacyChannelModels[0].Protocol || item.CapabilityConfigJSON != legacyChannelModels[0].CapabilityConfigJSON {
			return nil, errors.New("GPT Image 2 的旧 SKU 渠道、协议或能力配置不一致，拒绝自动合并")
		}
	}
	canonical, canonicalErr := repo.ChannelModelByKeyIncludingDisabled(channelID, "gpt-image-2")
	if canonicalErr != nil && !errors.Is(canonicalErr, gorm.ErrRecordNotFound) {
		return nil, canonicalErr
	}
	tiers, tierErr := image2PriceTiersFromLegacy(repo, legacyChannelModels)
	if tierErr != nil {
		return nil, tierErr
	}
	channelModel := model.ChannelModel{}
	alreadySynced := canonical != nil
	if canonical != nil {
		channelModel = *canonical
		channelModel.PriceTiers = tiers
	} else {
		modelID, idErr := repo.NextPrefixedID("MODEL")
		if idErr != nil {
			return nil, idErr
		}
		channelModel = model.ChannelModel{
			ID: modelID, ChannelID: channelID, ModelKey: "gpt-image-2", ProviderModelKey: "gpt-image-2", DisplayName: "GPT Image 2",
			Capability: "image", Protocol: legacyChannelModels[0].Protocol, Enabled: true, PriceConfigured: true, PriceVersion: 1,
			CapabilityConfigJSON: legacyChannelModels[0].CapabilityConfigJSON, CapabilityVersion: 1,
		}
	}
	applyPriceSummary(&channelModel, tiers)
	logicalModel := model.LogicalModel{Code: "image-2", Name: "GPT Image 2", Description: "GPT Image 2", Capability: "image", Enabled: true, SortOrder: legacyModels[0].SortOrder}
	creatingLogical := true
	if existing, found := byCode["image-2"]; found {
		logicalModel = existing
		creatingLogical = false
	}
	plan := &familyPlan{logicalModel: logicalModel, channelModel: channelModel, priceTiers: tiers, alreadySynced: alreadySynced, creatingLogical: creatingLogical}
	for index := range legacyModels {
		plan.legacyLogicalModelIDs = append(plan.legacyLogicalModelIDs, legacyModels[index].ID)
		plan.legacyChannelModelIDs = append(plan.legacyChannelModelIDs, legacyChannelModels[index].ID)
	}
	return plan, nil
}

func image2PriceTiersFromLegacy(repo *repository.Repository, items []model.ChannelModel) ([]model.ChannelModelPriceTier, error) {
	result := make([]model.ChannelModelPriceTier, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		quality := ""
		for _, candidate := range []string{"1k", "2k", "4k"} {
			if strings.HasSuffix(strings.ToLower(item.ModelKey), "-"+candidate) {
				quality = candidate
			}
		}
		if quality == "" || seen[quality] {
			return nil, fmt.Errorf("无法从旧渠道模型 %s 推导唯一的 GPT Image 2 质量档", item.ModelKey)
		}
		seen[quality] = true
		tier := firstActiveTier(item)
		id, err := repo.NextPrefixedID("PTIER")
		if err != nil {
			return nil, err
		}
		selector, key, err := model.CanonicalSKUSelector(map[string]string{"quality": quality})
		if err != nil {
			return nil, err
		}
		tier.ID, tier.ChannelModelID, tier.Selector, tier.SelectorKey, tier.SelectorJSON = id, "", selector, key, key
		tier.Resolution, tier.VideoSeconds = "*", 0
		tier.ProviderModelKey = firstNonEmpty(tier.ProviderModelKey, item.ProviderModelKey, "gpt-image-2")
		tier.PriceVersion = 1
		result = append(result, tier)
	}
	if len(result) != 3 {
		return nil, errors.New("GPT Image 2 缺少 1K、2K 或 4K 价格档")
	}
	return result, nil
}

func disableLegacySKURecords(db *gorm.DB, plan familyPlan) error {
	if len(plan.legacyChannelModelIDs) == 0 && len(plan.legacyLogicalModelIDs) == 0 {
		return nil
	}
	var active int64
	if len(plan.legacyChannelModelIDs) > 0 {
		if err := db.Model(&model.Task{}).Where("channel_model_id IN ? AND status IN ?", plan.legacyChannelModelIDs, []model.TaskStatus{model.TaskStatusQueued, model.TaskStatusRunning}).Count(&active).Error; err != nil {
			return err
		}
	}
	if active > 0 {
		return fmt.Errorf("仍有 %d 个排队或运行中的任务引用旧渠道 SKU，稍后重试迁移", active)
	}
	if len(plan.legacyLogicalModelIDs) > 0 {
		if err := db.Model(&model.LogicalModel{}).Where("id IN ?", plan.legacyLogicalModelIDs).Update("enabled", false).Error; err != nil {
			return err
		}
	}
	if len(plan.legacyChannelModelIDs) > 0 {
		if err := db.Model(&model.ChannelModel{}).Where("id IN ?", plan.legacyChannelModelIDs).Update("enabled", false).Error; err != nil {
			return err
		}
	}
	return nil
}

func buildFamilyPlan(repo *repository.Repository, logicalModel model.LogicalModel, graph *repository.LogicalModelGraph) (familyPlan, error) {
	active := make([]model.ChannelModel, 0, len(graph.Routes))
	byID := make(map[string]model.ChannelModel, len(graph.ChannelModels))
	for _, item := range graph.ChannelModels {
		byID[item.ID] = item
	}
	for _, route := range graph.Routes {
		if route.Enabled && route.Weight > 0 {
			if item, found := byID[route.ChannelModelID]; found {
				active = append(active, item)
			}
		}
	}
	if len(active) == 0 {
		return familyPlan{}, fmt.Errorf("前台模型 %s 没有启用供应线路", logicalModel.Code)
	}
	channelID := active[0].ChannelID
	for _, item := range active {
		if item.ChannelID != channelID || item.Capability != "video" {
			return familyPlan{}, fmt.Errorf("前台模型 %s 的供应线路不属于同一视频系统渠道，需人工处理", logicalModel.Code)
		}
	}
	canonical, err := repo.ChannelModelByKeyIncludingDisabled(channelID, logicalModel.Code)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return familyPlan{}, err
	}
	if canonical != nil {
		return familyPlan{logicalModel: logicalModel, channelModel: *canonical, priceTiers: canonical.PriceTiers}, nil
	}
	configJSON, err := mergedVideoCapabilityConfig(active)
	if err != nil {
		return familyPlan{}, fmt.Errorf("合并 %s 视频能力：%w", logicalModel.Code, err)
	}
	modelID, err := repo.NextPrefixedID("MODEL")
	if err != nil {
		return familyPlan{}, err
	}
	tiers, err := priceTiersFromLegacy(repo, active)
	if err != nil {
		return familyPlan{}, fmt.Errorf("读取 %s 旧 SKU 价格：%w", logicalModel.Code, err)
	}
	channelModel := model.ChannelModel{
		ID: modelID, ChannelID: channelID, ModelKey: logicalModel.Code, ProviderModelKey: active[0].ProviderModelKey,
		DisplayName: logicalModel.Name, Capability: "video", Protocol: active[0].Protocol, Enabled: true,
		PriceConfigured: len(tiers) > 0, PriceVersion: 1, CapabilityConfigJSON: configJSON, CapabilityVersion: 1,
	}
	applyPriceSummary(&channelModel, tiers)
	return familyPlan{logicalModel: logicalModel, channelModel: channelModel, priceTiers: tiers}, nil
}

func mergedVideoCapabilityConfig(items []model.ChannelModel) (string, error) {
	config, err := service.DecodeModelCapabilityConfig(items[0].CapabilityConfigJSON)
	if err != nil || config == nil || config.Video == nil {
		return "", errors.New("首个旧 SKU 缺少视频能力配置")
	}
	resolutions := make(map[string]bool)
	durations := make(map[int]bool)
	for _, item := range items {
		candidate, decodeErr := service.DecodeModelCapabilityConfig(item.CapabilityConfigJSON)
		if decodeErr != nil || candidate == nil || candidate.Video == nil {
			return "", fmt.Errorf("渠道模型 %s 的视频能力无效", item.ModelKey)
		}
		for _, value := range candidate.Video.Resolutions {
			resolutions[value] = true
		}
		for _, value := range candidate.Video.Duration.Values {
			durations[value] = true
		}
	}
	config.Video.Resolutions = sortedStrings(resolutions)
	if len(durations) > 0 {
		config.Video.Duration.Selection = "enum"
		config.Video.Duration.Values = sortedInts(durations)
		config.Video.Duration.Default = config.Video.Duration.Values[0]
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func priceTiersFromLegacy(repo *repository.Repository, items []model.ChannelModel) ([]model.ChannelModelPriceTier, error) {
	result := make([]model.ChannelModelPriceTier, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		resolution, seconds := legacyTierDimensions(item)
		key := resolution + fmt.Sprintf(":%d", seconds)
		if seen[key] {
			return nil, fmt.Errorf("旧 SKU 在规格 %s 上重复，无法决定唯一价格", key)
		}
		seen[key] = true
		tier := firstActiveTier(item)
		id, err := repo.NextPrefixedID("PTIER")
		if err != nil {
			return nil, err
		}
		selectorInput := map[string]string{}
		if resolution != "*" {
			selectorInput["vquality"] = resolution
		}
		if seconds > 0 {
			selectorInput["videoSeconds"] = strconv.Itoa(seconds)
		}
		selector, selectorKey, selectorErr := model.CanonicalSKUSelector(selectorInput)
		if selectorErr != nil {
			return nil, selectorErr
		}
		tier.ID, tier.ChannelModelID, tier.Selector, tier.SelectorKey, tier.SelectorJSON, tier.Resolution, tier.VideoSeconds = id, "", selector, selectorKey, selectorKey, resolution, seconds
		tier.ProviderModelKey = firstNonEmpty(tier.ProviderModelKey, item.ProviderModelKey, item.ModelKey)
		tier.PriceVersion = 1
		result = append(result, tier)
	}
	return result, nil
}

func legacyTierDimensions(item model.ChannelModel) (string, int) {
	resolution := "*"
	seconds := 0
	config, _ := service.DecodeModelCapabilityConfig(item.CapabilityConfigJSON)
	if config != nil && config.Video != nil {
		if len(config.Video.Resolutions) == 1 {
			resolution = strings.ToLower(config.Video.Resolutions[0])
		}
		if len(config.Video.Duration.Values) == 1 {
			seconds = config.Video.Duration.Values[0]
		}
	}
	for _, candidate := range []string{"480p", "720p", "1080p", "1440p", "2160p"} {
		if strings.Contains(strings.ToLower(item.ModelKey), candidate) {
			resolution = candidate
		}
	}
	return resolution, seconds
}

func firstActiveTier(item model.ChannelModel) model.ChannelModelPriceTier {
	for _, tier := range item.PriceTiers {
		if tier.Enabled && tier.PriceConfigured {
			return tier
		}
	}
	return model.ChannelModelPriceTier{ProviderModelKey: item.ProviderModelKey, BillingMode: item.BillingMode, UnitPriceMicrocredits: item.UnitPriceMicrocredits, InputTokenPriceMicrocredits: item.InputTokenPriceMicrocredits, OutputTokenPriceMicrocredits: item.OutputTokenPriceMicrocredits, CachedTokenPriceMicrocredits: item.CachedTokenPriceMicrocredits, PriceConfigured: item.PriceConfigured, Enabled: item.Enabled}
}

func applyPriceSummary(channelModel *model.ChannelModel, tiers []model.ChannelModelPriceTier) {
	if len(tiers) == 0 {
		return
	}
	tier := tiers[0]
	channelModel.BillingMode = tier.BillingMode
	channelModel.UnitPriceMicrocredits = tier.UnitPriceMicrocredits
	channelModel.InputTokenPriceMicrocredits = tier.InputTokenPriceMicrocredits
	channelModel.OutputTokenPriceMicrocredits = tier.OutputTokenPriceMicrocredits
	channelModel.CachedTokenPriceMicrocredits = tier.CachedTokenPriceMicrocredits
	channelModel.PriceConfigured = false
	for _, candidate := range tiers {
		if candidate.Enabled && candidate.PriceConfigured {
			channelModel.PriceConfigured = true
			break
		}
	}
}

func migrationAdmin(db *gorm.DB) (*model.User, error) {
	var actor model.User
	if err := db.Where("role = ?", model.UserRoleAdmin).Order("created_at asc").First(&actor).Error; err != nil {
		return nil, fmt.Errorf("需要现有管理员作为迁移审计主体：%w", err)
	}
	return &actor, nil
}

func decodeLegacyModelIDs(raw string) []string {
	var ids []string
	_ = json.Unmarshal([]byte(raw), &ids)
	return ids
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sortedStrings(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedInts(values map[int]bool) []int {
	result := make([]int, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}
