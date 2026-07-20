package ebeecontrol

// PlannedAction represents a single action in a response plan.
type PlannedAction struct {
	// ActionType is the kind of response to execute.
	ActionType ActionType
	// Target is the subject of the action (pod ID or namespace).
	Target string
	// Priority determines execution order (lower = higher priority).
	Priority int
}

// ResponsePlan is a complete set of actions generated from a threat assessment.
type ResponsePlan struct {
	AssessmentID   string
	Classification ThreatClassification
	Namespace      string
	PodID          string
	Actions        []PlannedAction
}

// GenerateResponsePlan builds a response plan based on the threat assessment.
//
// Rules:
//   - low:      no response actions (empty actions)
//   - medium:   deploy at least 2 additional honeytokens in same namespace
//   - high:     pod isolation + IP block + deploy at least 2 additional honeytokens
//   - critical: pod isolation + IP block + deploy at least 2 additional honeytokens
//
// Priority ordering (lower number = higher priority):
//  1. Pod isolation (most urgent containment)
//  2. IP block (prevent further access)
//  3. Additional honeytokens (expand detection surface)
func GenerateResponsePlan(assessment ThreatAssessment, namespace, podID string) ResponsePlan {
	plan := ResponsePlan{
		AssessmentID:   assessment.AssessmentID,
		Classification: assessment.Classification,
		Namespace:      namespace,
		PodID:          podID,
	}

	plan.Actions = buildActions(assessment.Classification, namespace, podID)
	return plan
}

// buildActions constructs the list of planned actions based on threat classification.
func buildActions(classification ThreatClassification, namespace, podID string) []PlannedAction {
	if classification == ThreatLow {
		return nil
	}

	var actions []PlannedAction

	// For high and critical: include pod isolation and IP block.
	if classification == ThreatHigh || classification == ThreatCritical {
		actions = append(actions, PlannedAction{
			ActionType: ActionPodIsolation,
			Target:     podID,
			Priority:   1,
		})
		actions = append(actions, PlannedAction{
			ActionType: ActionIPBlock,
			Target:     podID,
			Priority:   2,
		})
	}

	// For medium, high, and critical: deploy additional honeytokens.
	priority := 3
	if classification == ThreatMedium {
		priority = 1
	}
	actions = append(actions, PlannedAction{
		ActionType: ActionAdditionalHoneytokens,
		Target:     namespace,
		Priority:   priority,
	})

	return actions
}
