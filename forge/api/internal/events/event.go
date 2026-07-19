package events

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type EventType string

func (t EventType) Validate() error {
	if t == "" || t == WildcardEventType {
		return fmt.Errorf("invalid event type: %q", t)
	}
	return nil
}

const (
	EventServerCreated   EventType = "ServerCreated"
	EventServerDeleted   EventType = "ServerDeleted"
	EventServerStarted   EventType = "ServerStarted"
	EventServerStopped   EventType = "ServerStopped"
	EventServerRestarted EventType = "ServerRestarted"
	EventServerUpdated   EventType = "ServerUpdated"

	EventNodeOnline             EventType = "NodeOnline"
	EventNodeSuspected          EventType = "NodeSuspected"
	EventNodeUnreachable        EventType = "NodeUnreachable"
	EventNodeOffline            EventType = "NodeOffline"
	EventNodeDegraded           EventType = "NodeDegraded"
	EventNodeRecovered          EventType = "NodeRecovered"
	EventNodeDrainingStarted    EventType = "NodeDrainingStarted"
	EventNodeDrainingCompleted  EventType = "NodeDrainingCompleted"
	EventNodeMaintenanceStarted EventType = "NodeMaintenanceStarted"
	EventNodeMaintenanceEnded   EventType = "NodeMaintenanceEnded"
	EventNodeCapacityExceeded   EventType = "NodeCapacityExceeded"

	EventPlacementCreated     EventType = "PlacementCreated"
	EventReservationCreated   EventType = "ReservationCreated"
	EventReservationConfirmed EventType = "ReservationConfirmed"
	EventReservationExpired   EventType = "ReservationExpired"
	EventReservationCancelled EventType = "ReservationCancelled"
	EventDesiredStateChanged  EventType = "DesiredStateChanged"
	EventActualStateChanged   EventType = "ActualStateChanged"

	EventEvacuationPlanCreated       EventType = "EvacuationPlanCreated"
	EventEvacuationPlanCompleted     EventType = "EvacuationPlanCompleted"
	EventEvacuationPlanFailed        EventType = "EvacuationPlanFailed"
	EventEvacuationCandidateSelected EventType = "EvacuationCandidateSelected"

	EventRuntimeRegistered        EventType = "RuntimeRegistered"
	EventRuntimeUnavailable       EventType = "RuntimeUnavailable"
	EventRuntimeCapabilityChanged EventType = "RuntimeCapabilityChanged"

	EventMigrationCreated   EventType = "MigrationCreated"
	EventMigrationStarted   EventType = "MigrationStarted"
	EventMigrationCompleted EventType = "MigrationCompleted"
	EventMigrationFailed    EventType = "MigrationFailed"
	EventMigrationCancelled EventType = "MigrationCancelled"

	EventRecoveryPlanCreated   EventType = "RecoveryPlanCreated"
	EventRecoveryPlanPlanned   EventType = "RecoveryPlanPlanned"
	EventRecoveryPlanFailed    EventType = "RecoveryPlanFailed"
	EventRecoveryPlanCancelled EventType = "RecoveryPlanCancelled"
	EventRecoveryItemCreated   EventType = "RecoveryItemCreated"

	// Auto-scaling events
	EventScalingPolicyCreated EventType = "ScalingPolicyCreated"
	EventServerScaledUp       EventType = "ServerScaledUp"
	EventServerScaledDown     EventType = "ServerScaledDown"
	EventScalingError         EventType = "ScalingError"

	// Deployment events
	EventDeploymentStarted    EventType = "DeploymentStarted"
	EventDeploymentCompleted  EventType = "DeploymentCompleted"
	EventDeploymentRolledBack EventType = "DeploymentRolledBack"
	EventDeploymentCancelled  EventType = "DeploymentCancelled"

	// Failover events
	EventFailoverDetected        EventType = "FailoverDetected"
	EventFailoverActionTriggered EventType = "FailoverActionTriggered"
	EventNodeFailureNotified     EventType = "NodeFailureNotified"
	EventNodeEvacuationStarted   EventType = "NodeEvacuationStarted"
	EventNodeRestartInitiated    EventType = "NodeRestartInitiated"

	// Load balancer events
	EventTargetGroupCreated  EventType = "TargetGroupCreated"
	EventTargetGroupDeleted  EventType = "TargetGroupDeleted"
	EventTargetAdded         EventType = "TargetAdded"
	EventTargetRemoved       EventType = "TargetRemoved"
	EventTargetHealthChanged EventType = "TargetHealthChanged"

	// Crash events
	EventServerCrashed               EventType = "ServerCrashed"
	EventServerCrashAutoRestarted     EventType = "ServerCrashAutoRestarted"
	EventServerCrashThresholdReached  EventType = "ServerCrashThresholdReached"
	EventServerCrashRecovered         EventType = "ServerCrashRecovered"

	// User events
	EventUserCreated    EventType = "UserCreated"
	EventUserDeleted    EventType = "UserDeleted"
	EventUserUpdated    EventType = "UserUpdated"
	EventUserLoggedIn   EventType = "UserLoggedIn"
	EventUserLoggedOut  EventType = "UserLoggedOut"
	EventUserSuspended  EventType = "UserSuspended"
	EventUserReactivated EventType = "UserReactivated"

	// Server resource events
	EventServerResourcesAllocated   EventType = "ServerResourcesAllocated"
	EventServerSuspended            EventType = "ServerSuspended"
	EventServerUnsuspended          EventType = "ServerUnsuspended"
	EventServerInstallCompleted     EventType = "ServerInstallCompleted"
	EventServerBackupCreated        EventType = "ServerBackupCreated"
	EventServerBackupFailed         EventType = "ServerBackupFailed"
	EventServerBackupRestored       EventType = "ServerBackupRestored"

	// Allocation events
	EventAllocationCreated EventType = "AllocationCreated"
	EventAllocationRemoved EventType = "AllocationRemoved"
)

func (e Envelope) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("envelope id is required")
	}
	if err := e.Type.Validate(); err != nil {
		return fmt.Errorf("envelope type: %w", err)
	}
	if e.Source == "" {
		return fmt.Errorf("envelope source is required")
	}
	if e.ResourceType == "" {
		return fmt.Errorf("envelope resource_type is required")
	}
	if e.ResourceID == "" {
		return fmt.Errorf("envelope resource_id is required")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("envelope timestamp is required")
	}
	return nil
}

type Envelope struct {
	ID            string         `json:"id"`
	Type          EventType      `json:"type"`
	Timestamp     time.Time      `json:"timestamp"`
	Source        string         `json:"source"`
	ResourceType  string         `json:"resource_type"`
	ResourceID    string         `json:"resource_id"`
	CorrelationID string         `json:"correlation_id"`
	Payload       map[string]any `json:"payload"`
}

func NewEnvelope(eventType EventType, source, resourceType, resourceID string, payload map[string]any) Envelope {
	if payload == nil {
		payload = map[string]any{}
	}
	correlationID := correlationIDFromPayload(payload)
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	return Envelope{
		ID:            uuid.NewString(),
		Type:          eventType,
		Timestamp:     time.Now().UTC(),
		Source:        source,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		CorrelationID: correlationID,
		Payload:       payload,
	}
}

func correlationIDFromPayload(payload map[string]any) string {
	for _, key := range []string{"correlationId", "correlation_id"} {
		if value, ok := payload[key]; ok {
			if text, ok := value.(string); ok && text != "" {
				return text
			}
		}
	}
	return ""
}
