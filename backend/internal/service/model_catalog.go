package service

import (
	"encoding/json"
	"strings"

	"infinite-canvas/backend/internal/model"
)

type ModelCatalogSource string

const (
	ModelCatalogSourceFrontend ModelCatalogSource = "frontend"
	ModelCatalogSourceSystem   ModelCatalogSource = "system"
)

// ModelCatalogResponse 只包含创作端选择模型所需的公开数据，不暴露渠道密钥或上游地址。
type ModelCatalogResponse struct {
	Source   ModelCatalogSource     `json:"source"`
	Models   []PublicLogicalModel   `json:"models,omitempty"`
	Channels []PublicChannelCatalog `json:"channels,omitempty"`
}

type PublicChannelCatalog struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	DisplayName string               `json:"displayName"`
	Models      []PublicChannelModel `json:"models"`
}

type PublicChannelModel struct {
	ID               string                        `json:"id"`
	ModelKey         string                        `json:"modelKey"`
	DisplayName      string                        `json:"displayName"`
	Capability       string                        `json:"capability"`
	CapabilityConfig map[string]any                `json:"capabilityConfig,omitempty"`
	PriceTiers       []PublicChannelModelPriceTier `json:"priceTiers"`
	PricingMode      string                        `json:"pricingMode"`
	DisplayPrice     *int64                        `json:"displayPrice,omitempty"`
	PriceLabel       string                        `json:"priceLabel"`
	Available        bool                          `json:"available"`
}

type PublicChannelModelPriceTier struct {
	ID                           string            `json:"id"`
	Selector                     map[string]string `json:"selector,omitempty"`
	Resolution                   string            `json:"resolution"`
	VideoSeconds                 int               `json:"videoSeconds"`
	BillingMode                  string            `json:"billingMode"`
	UnitPriceMicrocredits        int64             `json:"unitPriceMicrocredits"`
	InputTokenPriceMicrocredits  int64             `json:"inputTokenPriceMicrocredits"`
	OutputTokenPriceMicrocredits int64             `json:"outputTokenPriceMicrocredits"`
	CachedTokenPriceMicrocredits int64             `json:"cachedTokenPriceMicrocredits"`
}

// ModelCatalog 在启用前台模型时返回逻辑模型；关闭时只回退到可用、已定价的系统模型。
func (s *Service) ModelCatalog(intent *ModelRequestIntent) (*ModelCatalogResponse, error) {
	frontendEnabled, err := s.FeatureEnabled(FeatureFrontendModels)
	if err != nil {
		return nil, err
	}
	if frontendEnabled {
		models, err := s.PublicLogicalModels(intent)
		if err != nil {
			return nil, err
		}
		return &ModelCatalogResponse{Source: ModelCatalogSourceFrontend, Models: models}, nil
	}
	channels, err := s.publicSystemChannelCatalog(intent)
	if err != nil {
		return nil, err
	}
	return &ModelCatalogResponse{Source: ModelCatalogSourceSystem, Channels: channels}, nil
}

func (s *Service) publicSystemChannelCatalog(intent *ModelRequestIntent) ([]PublicChannelCatalog, error) {
	channels, err := s.repo.SystemChannels(false)
	if err != nil {
		return nil, err
	}
	result := make([]PublicChannelCatalog, 0, len(channels))
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		channelModels, err := s.repo.ChannelModels(channel.ID, false)
		if err != nil {
			return nil, err
		}
		models := make([]PublicChannelModel, 0, len(channelModels))
		for index := range channelModels {
			channelModel := &channelModels[index]
			if !channelModel.Enabled || !channelModelMatchesCatalogIntent(channelModel, intent) {
				continue
			}
			publicModel, ok := publicSystemChannelModel(channelModel, channel.APIFormat)
			if ok {
				models = append(models, publicModel)
			}
		}
		if len(models) == 0 {
			continue
		}
		result = append(result, PublicChannelCatalog{ID: channel.ID, Name: channel.Name, DisplayName: channel.Name, Models: models})
	}
	return result, nil
}

func channelModelMatchesCatalogIntent(channelModel *model.ChannelModel, intent *ModelRequestIntent) bool {
	if intent == nil || strings.TrimSpace(intent.Capability) == "" {
		return true
	}
	if normalizeCapability(channelModel.Capability) != normalizeCapability(intent.Capability) {
		return false
	}
	tier := channelModelPriceTierForIntent(*channelModel, *intent)
	return tier != nil && validChannelModelPriceTier(*tier)
}

func publicSystemChannelModel(channelModel *model.ChannelModel, apiFormat string) (PublicChannelModel, bool) {
	tiers := make([]PublicChannelModelPriceTier, 0, len(channelModel.PriceTiers))
	for _, tier := range channelModel.PriceTiers {
		if !tier.Enabled || !tier.PriceConfigured || !validChannelModelPriceTier(tier) {
			continue
		}
		tiers = append(tiers, PublicChannelModelPriceTier{
			ID:                           tier.ID,
			Selector:                     model.DecodeSKUSelector(tier.SelectorJSON),
			Resolution:                   tier.Resolution,
			VideoSeconds:                 tier.VideoSeconds,
			BillingMode:                  tier.BillingMode,
			UnitPriceMicrocredits:        tier.UnitPriceMicrocredits,
			InputTokenPriceMicrocredits:  tier.InputTokenPriceMicrocredits,
			OutputTokenPriceMicrocredits: tier.OutputTokenPriceMicrocredits,
			CachedTokenPriceMicrocredits: tier.CachedTokenPriceMicrocredits,
		})
	}
	if len(tiers) == 0 {
		return PublicChannelModel{}, false
	}
	pricingMode, displayPrice, priceLabel := systemChannelPriceDisplay(tiers)
	return PublicChannelModel{
		ID:               channelModel.ID,
		ModelKey:         channelModel.ModelKey,
		DisplayName:      channelModel.DisplayName,
		Capability:       channelModel.Capability,
		CapabilityConfig: publicChannelModelCapabilityConfig(channelModel, apiFormat),
		PriceTiers:       tiers,
		PricingMode:      pricingMode,
		DisplayPrice:     displayPrice,
		PriceLabel:       priceLabel,
		Available:        true,
	}, true
}

func validChannelModelPriceTier(tier model.ChannelModelPriceTier) bool {
	switch strings.TrimSpace(tier.BillingMode) {
	case "fixed_request", "per_second":
		return tier.UnitPriceMicrocredits > 0
	case "token":
		return tier.InputTokenPriceMicrocredits > 0 || tier.OutputTokenPriceMicrocredits > 0 || tier.CachedTokenPriceMicrocredits > 0
	default:
		return false
	}
}

func systemChannelPriceDisplay(tiers []PublicChannelModelPriceTier) (string, *int64, string) {
	if len(tiers) != 1 {
		return "provider", nil, "按规格计费"
	}
	price := systemChannelTierDisplayPrice(tiers[0])
	if price <= 0 {
		return "provider", nil, "按规格计费"
	}
	return "provider", &price, ""
}

func systemChannelTierDisplayPrice(tier PublicChannelModelPriceTier) int64 {
	if tier.BillingMode == "fixed_request" || tier.BillingMode == "per_second" {
		return tier.UnitPriceMicrocredits
	}
	if tier.OutputTokenPriceMicrocredits > 0 {
		return tier.OutputTokenPriceMicrocredits
	}
	return tier.InputTokenPriceMicrocredits
}

func publicChannelModelCapabilityConfig(channelModel *model.ChannelModel, apiFormat string) map[string]any {
	if strings.TrimSpace(channelModel.CapabilityConfigJSON) == "" {
		return nil
	}
	config, err := DecodeModelCapabilityConfig(channelModel.CapabilityConfigJSON)
	if err != nil || config == nil {
		return nil
	}
	normalized, err := NormalizeModelCapabilityConfig(channelModel.Capability, string(channelModel.Protocol), firstNonEmpty(channelModel.ProviderModelKey, channelModel.ModelKey), apiFormat, config)
	if err != nil || normalized == nil {
		return nil
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil
	}
	result := map[string]any{}
	if json.Unmarshal(encoded, &result) != nil {
		return nil
	}
	return result
}
