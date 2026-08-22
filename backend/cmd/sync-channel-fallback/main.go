package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"

	"gorm.io/gorm"
)

// sync-channel-fallback 将一个系统渠道的模型、规格价格和前台线路投影到备用渠道。
// 它不会改写历史任务、账单或既有 revision；新任务才会从新 revision 使用备用线路。
func main() {
	apply := flag.Bool("apply", false, "写入备用渠道模型和前台备用线路")
	sourceURL := flag.String("source-url", "https://api.yeellow.com", "主渠道 Base URL")
	targetURL := flag.String("target-url", "https://www.ai-wave.org", "备用渠道 Base URL")
	targetName := flag.String("target-name", "AI Wave", "备用渠道名称")
	apiKeyEnv := flag.String("api-key-env", "AI_WAVE_API_KEY", "备用渠道 API Key 的环境变量名")
	flag.Parse()

	db, err := database.Open(database.Config{Driver: "postgres", DSN: os.Getenv("DATABASE_URL"), DataDir: os.Getenv("CANVAS_BACKEND_DATA_DIR")})
	if err != nil {
		log.Fatal(err)
	}
	if db.Dialector.Name() != "postgres" {
		log.Fatal("备用渠道同步只允许连接 PostgreSQL")
	}
	if err := database.ConfigurePool(db); err != nil {
		log.Fatal(err)
	}
	if err := database.MigrateSchema(db); err != nil {
		log.Fatal(err)
	}

	repo := repository.New(db)
	source, target, err := resolveChannels(repo, *sourceURL, *targetURL)
	if err != nil {
		log.Fatal(err)
	}
	sourceModels, err := repo.ChannelModels(source.ID, false)
	if err != nil {
		log.Fatal(err)
	}
	if len(sourceModels) == 0 {
		log.Fatal("主渠道没有启用的系统模型")
	}

	legacyTierCount := countLegacyPriceTierRepairs(sourceModels)
	fallbackCount, err := countFallbackRoutes(repo, sourceModels, target)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("主渠道=%s (%s)，备用渠道=%s，启用模型=%d，待补系统规格价格=%d，待新增前台备用线路=%d", source.Name, source.BaseURL, targetLabel(target), len(sourceModels), legacyTierCount, fallbackCount)
	if !*apply {
		log.Print("dry-run 完成；确认后使用 --apply。不会删除主渠道模型或既有前台线路")
		return
	}

	apiKey := strings.TrimSpace(os.Getenv(*apiKeyEnv))
	if apiKey == "" {
		log.Fatalf("请通过 %s 提供备用渠道 API Key", *apiKeyEnv)
	}
	actor, err := migrationAdmin(db)
	if err != nil {
		log.Fatal(err)
	}
	svc := service.New(repo, os.Getenv("CANVAS_BACKEND_DATA_DIR"))
	target, err = upsertTargetChannel(svc, repo, actor, target, *targetURL, *targetName, apiKey, source, sourceModels)
	if err != nil {
		log.Fatal(err)
	}

	targetBySourceID := make(map[string]model.ChannelModel, len(sourceModels))
	for index := range sourceModels {
		if repaired, repairErr := materializeLegacyPriceTier(repo, &sourceModels[index]); repairErr != nil {
			log.Fatalf("补齐 %s 的系统规格价格失败：%v", sourceModels[index].ModelKey, repairErr)
		} else if repaired {
			log.Printf("已补齐 %s 的通配系统规格价格", sourceModels[index].ModelKey)
		}
		cloned, cloneErr := syncChannelModel(repo, sourceModels[index], target.ID)
		if cloneErr != nil {
			log.Fatalf("同步备用模型 %s 失败：%v", sourceModels[index].ModelKey, cloneErr)
		}
		targetBySourceID[sourceModels[index].ID] = cloned
	}

	updated, err := appendFallbackRoutes(svc, actor, repo, targetBySourceID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("同步完成：备用渠道=%s，模型=%d，前台模型新增备用线路=%d", target.Name, len(targetBySourceID), updated)
}

func resolveChannels(repo *repository.Repository, sourceURL string, targetURL string) (model.ModelChannel, *model.ModelChannel, error) {
	channels, err := repo.SystemChannels(true)
	if err != nil {
		return model.ModelChannel{}, nil, err
	}
	sourceMatches := make([]model.ModelChannel, 0, 1)
	var target *model.ModelChannel
	for index := range channels {
		channel := channels[index]
		switch normalizeBaseURL(channel.BaseURL) {
		case normalizeBaseURL(sourceURL):
			sourceMatches = append(sourceMatches, channel)
		case normalizeBaseURL(targetURL):
			copy := channel
			target = &copy
		}
	}
	if len(sourceMatches) != 1 {
		return model.ModelChannel{}, nil, fmt.Errorf("主渠道 URL %s 匹配到 %d 条系统渠道，拒绝猜测", sourceURL, len(sourceMatches))
	}
	if normalizeBaseURL(sourceURL) == normalizeBaseURL(targetURL) {
		return model.ModelChannel{}, nil, errors.New("主渠道和备用渠道 URL 不能相同")
	}
	return sourceMatches[0], target, nil
}

func upsertTargetChannel(svc *service.Service, repo *repository.Repository, actor *model.User, target *model.ModelChannel, targetURL string, targetName string, apiKey string, source model.ModelChannel, sourceModels []model.ChannelModel) (*model.ModelChannel, error) {
	modelKeys := make([]string, 0, len(sourceModels))
	for _, item := range sourceModels {
		modelKeys = append(modelKeys, item.ModelKey)
	}
	enabled := true
	useGlobalConcurrency := source.ConcurrencyLimit == 0
	request := service.ChannelRequest{
		Name:                 strings.TrimSpace(targetName),
		BaseURL:              strings.TrimRight(strings.TrimSpace(targetURL), "/"),
		APIKey:               apiKey,
		Models:               modelKeys,
		Headers:              []service.OutboundHeader{},
		Enabled:              &enabled,
		UseGlobalConcurrency: &useGlobalConcurrency,
	}
	if !useGlobalConcurrency {
		limit := source.ConcurrencyLimit
		request.ConcurrencyLimit = &limit
	}
	if target == nil {
		created, err := svc.CreateSystemChannel(actor, request)
		if err != nil {
			return nil, err
		}
		return repo.AdminSystemChannel(created.ID)
	}
	if _, err := svc.UpdateSystemChannel(actor, target.ID, request); err != nil {
		return nil, err
	}
	return repo.AdminSystemChannel(target.ID)
}

func materializeLegacyPriceTier(repo *repository.Repository, item *model.ChannelModel) (bool, error) {
	if item == nil || !item.PriceConfigured || hasActivePriceTier(*item) {
		return false, nil
	}
	selector, selectorKey, err := model.CanonicalSKUSelector(map[string]string{})
	if err != nil {
		return false, err
	}
	selectorJSON, err := json.Marshal(selector)
	if err != nil {
		return false, err
	}
	tierID, err := repo.NextPrefixedID("PTIER")
	if err != nil {
		return false, err
	}
	now := time.Now()
	item.PriceVersion++
	item.UpdatedAt = now
	tier := model.ChannelModelPriceTier{
		ID:                           tierID,
		ChannelModelID:               item.ID,
		SelectorKey:                  selectorKey,
		SelectorJSON:                 string(selectorJSON),
		Resolution:                   "*",
		ProviderModelKey:             firstNonEmpty(item.ProviderModelKey, item.ModelKey),
		BillingMode:                  item.BillingMode,
		UnitPriceMicrocredits:        item.UnitPriceMicrocredits,
		InputTokenPriceMicrocredits:  item.InputTokenPriceMicrocredits,
		OutputTokenPriceMicrocredits: item.OutputTokenPriceMicrocredits,
		CachedTokenPriceMicrocredits: item.CachedTokenPriceMicrocredits,
		PriceConfigured:              true,
		Enabled:                      true,
		PriceVersion:                 item.PriceVersion,
		CreatedAt:                    now,
		UpdatedAt:                    now,
	}
	item.PriceTiers = append(item.PriceTiers, tier)
	if err := repo.SaveChannelModelWithPriceTiers(item, item.PriceTiers); err != nil {
		return false, err
	}
	return true, nil
}

func syncChannelModel(repo *repository.Repository, source model.ChannelModel, targetChannelID string) (model.ChannelModel, error) {
	existing, err := repo.ChannelModelByKeyIncludingDisabled(targetChannelID, source.ModelKey)
	creating := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !creating {
		return model.ChannelModel{}, err
	}
	now := time.Now()
	clone := source
	clone.ChannelID = targetChannelID
	clone.UpdatedAt = now
	if creating {
		clone.ID, err = repo.NextPrefixedID("MODEL")
		if err != nil {
			return model.ChannelModel{}, err
		}
		clone.CreatedAt = now
	} else {
		clone.ID = existing.ID
		clone.CreatedAt = existing.CreatedAt
		clone.PriceVersion = max(existing.PriceVersion+1, source.PriceVersion)
	}
	tiers := make([]model.ChannelModelPriceTier, 0, len(source.PriceTiers))
	for _, sourceTier := range source.PriceTiers {
		tier := sourceTier
		tier.ID, err = repo.NextPrefixedID("PTIER")
		if err != nil {
			return model.ChannelModel{}, err
		}
		tier.ChannelModelID = clone.ID
		tier.CreatedAt = now
		tier.UpdatedAt = now
		tiers = append(tiers, tier)
	}
	if err := repo.SaveChannelModelWithPriceTiers(&clone, tiers); err != nil {
		return model.ChannelModel{}, err
	}
	clone.PriceTiers = tiers
	return clone, nil
}

func countLegacyPriceTierRepairs(items []model.ChannelModel) int {
	count := 0
	for _, item := range items {
		if item.PriceConfigured && !hasActivePriceTier(item) {
			count++
		}
	}
	return count
}

func hasActivePriceTier(item model.ChannelModel) bool {
	for _, tier := range item.PriceTiers {
		if tier.Enabled && tier.PriceConfigured {
			return true
		}
	}
	return false
}

func countFallbackRoutes(repo *repository.Repository, sourceModels []model.ChannelModel, target *model.ModelChannel) (int, error) {
	sourceIDs := make(map[string]bool, len(sourceModels))
	targetBySourceID := make(map[string]string, len(sourceModels))
	for _, item := range sourceModels {
		sourceIDs[item.ID] = true
	}
	if target != nil {
		targetModels, err := repo.ChannelModels(target.ID, true)
		if err != nil {
			return 0, err
		}
		targetByKey := make(map[string]string, len(targetModels))
		for _, item := range targetModels {
			targetByKey[item.ModelKey] = item.ID
		}
		for _, source := range sourceModels {
			targetBySourceID[source.ID] = targetByKey[source.ModelKey]
		}
	}
	items, err := repo.LogicalModels(false)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range items {
		graph, graphErr := repo.LogicalModelGraph(item.ID, false)
		if graphErr != nil || graph.Revision == nil {
			return 0, fmt.Errorf("读取前台模型 %s 失败：%w", item.Code, graphErr)
		}
		existingRoutes := make(map[string]bool, len(graph.Routes))
		for _, route := range graph.Routes {
			existingRoutes[route.ChannelModelID] = true
		}
		for _, route := range graph.Routes {
			if !sourceIDs[route.ChannelModelID] {
				continue
			}
			if targetID := targetBySourceID[route.ChannelModelID]; targetID == "" || !existingRoutes[targetID] {
				count++
			}
		}
	}
	return count, nil
}

func appendFallbackRoutes(svc *service.Service, actor *model.User, repo *repository.Repository, targetBySourceID map[string]model.ChannelModel) (int, error) {
	items, err := repo.LogicalModels(false)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, item := range items {
		graph, graphErr := repo.LogicalModelGraph(item.ID, false)
		if graphErr != nil || graph.Revision == nil {
			return 0, fmt.Errorf("读取前台模型 %s 失败：%w", item.Code, graphErr)
		}
		existingTargetRoutes := make(map[string]bool, len(graph.Routes))
		for _, route := range graph.Routes {
			existingTargetRoutes[route.ChannelModelID] = true
		}
		routes := make([]service.LogicalRouteRequest, 0, len(graph.Routes)+1)
		changed := false
		for _, route := range graph.Routes {
			routes = append(routes, service.LogicalRouteRequest{ChannelModelID: route.ChannelModelID, Enabled: route.Enabled, Priority: route.Priority, Weight: route.Weight})
			fallback, sourceOK := targetBySourceID[route.ChannelModelID]
			if !sourceOK || existingTargetRoutes[fallback.ID] {
				continue
			}
			// 主线路的请求失败后已被标记为 tried；备用线路即使优先级更低也会成为剩余候选。
			routes = append(routes, service.LogicalRouteRequest{ChannelModelID: fallback.ID, Enabled: route.Enabled, Priority: route.Priority - 1, Weight: route.Weight})
			existingTargetRoutes[fallback.ID] = true
			changed = true
		}
		if !changed {
			continue
		}
		request, requestErr := logicalModelRequest(item, graph, routes)
		if requestErr != nil {
			return 0, fmt.Errorf("构造前台模型 %s 的备用线路失败：%w", item.Code, requestErr)
		}
		if _, saveErr := svc.SaveAdminLogicalModel(actor, item.ID, request); saveErr != nil {
			return 0, fmt.Errorf("保存前台模型 %s 的备用线路失败：%w", item.Code, saveErr)
		}
		updated++
		log.Printf("已为前台模型 %s 追加 AI Wave 备用线路", item.Code)
	}
	return updated, nil
}

func logicalModelRequest(item model.LogicalModel, graph *repository.LogicalModelGraph, routes []service.LogicalRouteRequest) (service.LogicalModelRequest, error) {
	spec, err := service.DecodeCapabilitySpec(graph.Revision.CapabilitySpecJSON)
	if err != nil {
		return service.LogicalModelRequest{}, err
	}
	defaults := map[string]any{}
	if raw := strings.TrimSpace(graph.Revision.DefaultOptionsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &defaults); err != nil {
			return service.LogicalModelRequest{}, err
		}
	}
	legacy := []string{}
	if raw := strings.TrimSpace(item.LegacyModelIDsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
			return service.LogicalModelRequest{}, err
		}
	}
	return service.LogicalModelRequest{
		Code:                    item.Code,
		Name:                    item.Name,
		Icon:                    item.Icon,
		Description:             item.Description,
		Capability:              item.Capability,
		Enabled:                 item.Enabled,
		SortOrder:               item.SortOrder,
		PricePolicy:             item.PricePolicy,
		BillingMode:             item.BillingMode,
		UnitPriceMicrocredits:   item.UnitPriceMicrocredits,
		InputPriceMicrocredits:  item.InputPriceMicrocredits,
		OutputPriceMicrocredits: item.OutputPriceMicrocredits,
		CachedPriceMicrocredits: item.CachedPriceMicrocredits,
		LegacyModelIDs:          legacy,
		CapabilitySpec:          spec,
		DefaultOptions:          defaults,
		Routes:                  routes,
		SourceChannelModelID:    item.SourceChannelModelID,
	}, nil
}

func migrationAdmin(db *gorm.DB) (*model.User, error) {
	var actor model.User
	if err := db.Where("role = ?", model.UserRoleAdmin).Order("created_at asc").First(&actor).Error; err != nil {
		return nil, fmt.Errorf("需要现有管理员作为同步审计主体：%w", err)
	}
	return &actor, nil
}

func normalizeBaseURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(strings.ToLower(strings.TrimSpace(raw)), "/")
	}
	path := strings.TrimRight(strings.ToLower(parsed.Path), "/")
	if path == "/v1" || path == "/v1beta" {
		path = ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host + path)
}

func targetLabel(target *model.ModelChannel) string {
	if target == nil {
		return "尚未创建"
	}
	return target.Name + " (" + target.BaseURL + ")"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
