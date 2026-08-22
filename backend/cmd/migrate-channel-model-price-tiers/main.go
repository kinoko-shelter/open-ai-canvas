package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
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
	logicalModel  model.LogicalModel
	channelModel  model.ChannelModel
	priceTiers    []model.ChannelModelPriceTier
	alreadySynced bool
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
			tiers = append(tiers, fmt.Sprintf("%s/%ds=>%s", tier.Resolution, tier.VideoSeconds, tier.ProviderModelKey))
		}
		log.Printf("%s: 系统模型=%s 价格档=[%s] 已同步=%t", plan.logicalModel.Code, plan.channelModel.ModelKey, strings.Join(tiers, ", "), plan.alreadySynced)
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
		if _, err := svc.SaveAdminLogicalModel(actor, plan.logicalModel.ID, service.LogicalModelRequest{
			Code: plan.logicalModel.Code, Name: plan.logicalModel.Name, Icon: plan.logicalModel.Icon, Description: plan.logicalModel.Description,
			Capability: plan.channelModel.Capability, Enabled: plan.logicalModel.Enabled, SortOrder: plan.logicalModel.SortOrder,
			LegacyModelIDs: decodeLegacyModelIDs(plan.logicalModel.LegacyModelIDsJSON), SourceChannelModelID: plan.channelModel.ID,
		}); err != nil {
			log.Fatalf("同步前台模型 %s 失败：%v", plan.logicalModel.Code, err)
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
	return plans, nil
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
		tier.ID, tier.ChannelModelID, tier.Resolution, tier.VideoSeconds = id, "", resolution, seconds
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
