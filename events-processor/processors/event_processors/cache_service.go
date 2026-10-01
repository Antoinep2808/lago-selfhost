package event_processors

import (
	"github.com/getlago/lago/events-processor/models"
	"github.com/getlago/lago/events-processor/utils"
)

type CacheService struct {
	chargeCacheStore *models.ChargeCache
}

func NewCacheService(chargeCacheStore *models.ChargeCache) *CacheService {
	return &CacheService{
		chargeCacheStore: chargeCacheStore,
	}
}

func (s *CacheService) ExpireCache(events []*models.EnrichedEvent) {
	cacheEntriesExpired := 0
	for _, event := range events {
		if event.FlatFilter == nil {
			continue
		}

		cacheResult := s.chargeCacheStore.Expire(event.FlatFilter, event.SubscriptionID)
		if cacheResult.Failure() {
			utils.CaptureError(cacheResult.Error())
		} else {
			cacheEntriesExpired++
		}
	}

	// Pendo Track: charge cache entries expired after event processing
	if cacheEntriesExpired > 0 && len(events) > 0 {
		ev := events[0]
		chargeID := ""
		if ev.ChargeID != nil {
			chargeID = *ev.ChargeID
		}
		chargeFilterID := ""
		if ev.ChargeFilterID != nil {
			chargeFilterID = *ev.ChargeFilterID
		}
		utils.PendoTrack("charge_cache_expired", "system", ev.OrganizationID, map[string]any{
			"organization_id":       ev.OrganizationID,
			"subscription_id":       ev.SubscriptionID,
			"plan_id":               ev.PlanID,
			"charge_id":             chargeID,
			"charge_filter_id":      chargeFilterID,
			"code":                  ev.Code,
			"cache_entries_expired": cacheEntriesExpired,
		})
	}
}
