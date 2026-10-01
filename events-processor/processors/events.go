package processors

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"

	tracer "github.com/getlago/lago/events-processor/config"
	"github.com/getlago/lago/events-processor/models"
	"github.com/getlago/lago/events-processor/utils"
)

func processEvents(records []*kgo.Record) []*kgo.Record {
	ctx := context.Background()
	span := tracer.GetTracerSpan(ctx, "post_process", "PostProcess.ProcessEvents")
	recordsAttr := attribute.Int("records.length", len(records))
	span.SetAttributes(recordsAttr)
	defer span.End()

	wg := sync.WaitGroup{}
	wg.Add(len(records))

	var mu sync.Mutex
	processedRecords := make([]*kgo.Record, 0)

	for _, record := range records {
		go func(record *kgo.Record) {
			defer wg.Done()

			sp := tracer.GetTracerSpan(ctx, "post_process", "PostProcess.ProcessOneEvent")
			defer sp.End()

			event := models.Event{}
			err := json.Unmarshal(record.Value, &event)
			if err != nil {
				logger.Error("Error unmarshalling message", slog.String("error", err.Error()))
				utils.CaptureError(err)

				mu.Lock()
				// If we fail to unmarshal the record, we should commit it as it will failed forever
				processedRecords = append(processedRecords, record)
				mu.Unlock()
				return
			}

			result := processEvent(&event)
			if result.Failure() {
				logger.Error(
					result.ErrorMessage(),
					slog.String("error_code", result.ErrorCode()),
					slog.String("error", result.ErrorMsg()),
				)

				if result.IsCapturable() {
					utils.CaptureErrorResultWithExtra(result, "event", event)
				}

				if result.IsRetryable() && time.Since(event.IngestedAt.Time()) < 12*time.Hour {
					// For retryable errors, we should avoid commiting the record,
					// It will be consumed again and reprocessed
					// Events older than 12 hours should also be pushed dead letter queue
					return
				}

				// Push failed records to the dead letter queue
				go processor.ProducerService.ProduceToDeadLetterQueue(ctx, event, result)

				// Pendo Track: usage event sent to dead letter queue
				utils.PendoTrack("usage_event_dead_lettered", "system", event.OrganizationID, map[string]any{
					"organization_id":          event.OrganizationID,
					"external_subscription_id": event.ExternalSubscriptionID,
					"code":                     event.Code,
					"error_code":               result.ErrorCode(),
					"error_message":            result.ErrorMessage(),
					"is_retryable":             result.IsRetryable(),
					"source":                   event.Source,
					"event_age_hours":          time.Since(event.IngestedAt.Time()).Hours(),
				})
			}

			// Track processed records
			mu.Lock()
			processedRecords = append(processedRecords, record)
			mu.Unlock()
		}(record)
	}

	wg.Wait()

	return processedRecords
}

func processEvent(event *models.Event) utils.Result[*models.EnrichedEvent] {
	enrichedEventResult := processor.EnrichmentService.EnrichEvent(event)
	if enrichedEventResult.Failure() {
		return failedResult(enrichedEventResult, enrichedEventResult.ErrorCode(), enrichedEventResult.ErrorMessage())
	}

	enrichedEvents := enrichedEventResult.Value()
	enrichedEvent := enrichedEvents[0]

	go processor.ProducerService.ProduceEnrichedEvent(ctx, enrichedEvent)

	// TODO(pre-aggregation): Uncomment to enable the feature
	// for _, ev := range enrichedEvents {
	// 	go processor.ProducerService.ProduceEnrichedExpendedEvent(ctx, ev)
	// }

	// Pendo Track: usage event successfully enriched and produced
	hasChargeFilters := false
	for _, ev := range enrichedEvents {
		if ev.FlatFilter != nil {
			hasChargeFilters = true
			break
		}
	}
	utils.PendoTrack("usage_event_enriched", "system", enrichedEvent.OrganizationID, map[string]any{
		"organization_id":          enrichedEvent.OrganizationID,
		"external_subscription_id": enrichedEvent.ExternalSubscriptionID,
		"subscription_id":          enrichedEvent.SubscriptionID,
		"plan_id":                  enrichedEvent.PlanID,
		"code":                     enrichedEvent.Code,
		"aggregation_type":         enrichedEvent.AggregationType,
		"source":                   enrichedEvent.Source,
		"has_charge_filters":       hasChargeFilters,
		"charge_count":             len(enrichedEvents),
	})

	if enrichedEvent.Subscription != nil && event.NotAPIPostProcessed() {
		payInAdvance := false
		for _, ev := range enrichedEvents {
			if ev.FlatFilter != nil && ev.FlatFilter.PayInAdvance {
				payInAdvance = true
				break
			}
		}

		if payInAdvance {
			go processor.ProducerService.ProduceChargedInAdvanceEvent(ctx, enrichedEvent)

			// Pendo Track: pay-in-advance billing triggered
			chargeID := ""
			if enrichedEvent.ChargeID != nil {
				chargeID = *enrichedEvent.ChargeID
			}
			chargeFilterID := ""
			if enrichedEvent.ChargeFilterID != nil {
				chargeFilterID = *enrichedEvent.ChargeFilterID
			}
			utils.PendoTrack("pay_in_advance_event_triggered", "system", enrichedEvent.OrganizationID, map[string]any{
				"organization_id":          enrichedEvent.OrganizationID,
				"external_subscription_id": enrichedEvent.ExternalSubscriptionID,
				"subscription_id":          enrichedEvent.SubscriptionID,
				"plan_id":                  enrichedEvent.PlanID,
				"code":                     enrichedEvent.Code,
				"aggregation_type":         enrichedEvent.AggregationType,
				"charge_id":                chargeID,
				"charge_filter_id":         chargeFilterID,
			})
		}

		flagResult := processor.RefreshService.FlagSubscriptionRefresh(enrichedEvent)
		if flagResult.Failure() {
			return failedResult(flagResult, "flag_subscription_refresh", "Error flagging subscription refresh")
		}

		// Pendo Track: subscription refresh flagged in Redis
		utils.PendoTrack("subscription_refresh_flagged", "system", enrichedEvent.OrganizationID, map[string]any{
			"organization_id":          enrichedEvent.OrganizationID,
			"subscription_id":          enrichedEvent.SubscriptionID,
			"external_subscription_id": enrichedEvent.ExternalSubscriptionID,
			"plan_id":                  enrichedEvent.PlanID,
			"code":                     enrichedEvent.Code,
		})

		// Expire cache at charge and charge filter level
		processor.CacheService.ExpireCache(enrichedEvents)
	}

	return utils.SuccessResult(enrichedEvent)
}

func failedResult(r utils.AnyResult, code string, message string) utils.Result[*models.EnrichedEvent] {
	result := utils.FailedResult[*models.EnrichedEvent](r.Error()).AddErrorDetails(code, message)
	result.Retryable = r.IsRetryable()
	result.Capture = r.IsCapturable()
	return result
}
