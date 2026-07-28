package cli

import (
	"flag"
	"fmt"
	"strings"
)

func (a App) missionHistory(args []string) int {
	var historyPath string
	var routeFilter, statusFilter, queryFilter string
	var jsonOut, compact, pilotReadiness bool
	fs := flag.NewFlagSet("mission history", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&historyPath, "history", "", "path to AO Mission route history JSON")
	fs.StringVar(&routeFilter, "route", "", "filter route-history entries by route")
	fs.StringVar(&statusFilter, "status-filter", "", "filter route-history entries by status")
	fs.StringVar(&queryFilter, "query", "", "filter route-history entries by reason or exact next action")
	fs.BoolVar(&pilotReadiness, "pilot-readiness", false, "filter route-history entries to pilot readiness timeline events")
	fs.BoolVar(&compact, "compact", false, "emit compact filtered timeline summary")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(historyPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission history: --history is required")
		return 2
	}
	summary, err := readMissionRouteHistory(historyPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission history: %v\n", err)
		return 1
	}
	summary = filterMissionRouteHistory(summary, missionHistoryFilters{
		Route:          routeFilter,
		Status:         statusFilter,
		Query:          queryFilter,
		PilotReadiness: pilotReadiness,
	})
	if jsonOut {
		return a.writeJSON(summary)
	}
	if compact {
		fmt.Fprintf(a.Stdout, "compact_timeline=mission=%s route_count=%d total_route_count=%d latest_route=%s filters=%s\n", summary.MissionID, summary.RouteCount, summary.TotalRouteCount, summary.LatestRoute, missionHistoryFilterLabel(summary))
		fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
		fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
		fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
		fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
		fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
		return 0
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_history=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "route_count=%d\n", summary.RouteCount)
	if summary.TotalRouteCount > 0 && summary.TotalRouteCount != summary.RouteCount {
		fmt.Fprintf(a.Stdout, "total_route_count=%d\n", summary.TotalRouteCount)
	}
	fmt.Fprintf(a.Stdout, "latest_route=%s\n", summary.LatestRoute)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionRouteHistoryItem struct {
	Schema              string `json:"schema"`
	MissionID           string `json:"mission_id"`
	Status              string `json:"status"`
	Route               string `json:"route"`
	Reason              string `json:"reason"`
	OperatorMode        string `json:"operator_mode"`
	SafeToRequest       bool   `json:"safe_to_request"`
	SafeToExecute       bool   `json:"safe_to_execute"`
	ExecutesWork        bool   `json:"executes_work"`
	ApprovesWork        bool   `json:"approves_work"`
	MutatesRepositories bool   `json:"mutates_repositories"`
	ExactNextAction     string `json:"exact_next_action"`
}

type missionHistorySummary struct {
	CommandSchemaVersion string                    `json:"command_schema_version"`
	Schema               string                    `json:"schema"`
	MissionID            string                    `json:"mission_id"`
	Status               string                    `json:"status"`
	OperatorMode         string                    `json:"operator_mode"`
	RouteCount           int                       `json:"route_count"`
	TotalRouteCount      int                       `json:"total_route_count,omitempty"`
	LatestRoute          string                    `json:"latest_route"`
	RouteFilter          string                    `json:"route_filter,omitempty"`
	StatusFilter         string                    `json:"status_filter,omitempty"`
	QueryFilter          string                    `json:"query_filter,omitempty"`
	PilotReadinessFilter bool                      `json:"pilot_readiness_filter,omitempty"`
	Routes               []missionRouteHistoryItem `json:"routes"`
	SafeToExecute        bool                      `json:"safe_to_execute"`
	ExecutesWork         bool                      `json:"executes_work"`
	ApprovesWork         bool                      `json:"approves_work"`
	MutatesRepositories  bool                      `json:"mutates_repositories"`
	ExactNextAction      string                    `json:"exact_next_action"`
}

type missionHistoryFilters struct {
	Route          string
	Status         string
	Query          string
	PilotReadiness bool
}

func readMissionRouteHistory(path string) (missionHistorySummary, error) {
	var input []missionRouteHistoryItem
	if err := readJSONFile(path, &input); err != nil {
		return missionHistorySummary{}, err
	}
	if len(input) == 0 {
		return missionHistorySummary{}, fmt.Errorf("mission history requires at least one route decision")
	}
	missionID := input[0].MissionID
	for i, item := range input {
		if item.Schema != "ao.mission.route-decision.v0.1" {
			return missionHistorySummary{}, fmt.Errorf("mission history item %d schema must be ao.mission.route-decision.v0.1", i)
		}
		if item.MissionID == "" || item.Status == "" || item.Route == "" || item.OperatorMode == "" || item.ExactNextAction == "" {
			return missionHistorySummary{}, fmt.Errorf("mission history item %d requires mission_id, status, route, operator_mode, and exact_next_action", i)
		}
		if item.MissionID != missionID {
			return missionHistorySummary{}, fmt.Errorf("mission history mission_id mismatch")
		}
		if item.OperatorMode != operatorMode {
			return missionHistorySummary{}, fmt.Errorf("mission history item %d operator_mode must be %s", i, operatorMode)
		}
		if item.SafeToExecute || item.ExecutesWork || item.ApprovesWork || item.MutatesRepositories {
			return missionHistorySummary{}, fmt.Errorf("mission history must not claim execution, approval, or repository mutation authority")
		}
	}
	latest := input[len(input)-1]
	return missionHistorySummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               "ao.command.mission-history.v0.1",
		MissionID:            missionID,
		Status:               "ready",
		OperatorMode:         operatorMode,
		RouteCount:           len(input),
		LatestRoute:          latest.Route,
		Routes:               append([]missionRouteHistoryItem(nil), input...),
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      latest.ExactNextAction,
	}, nil
}

func filterMissionRouteHistory(summary missionHistorySummary, filters missionHistoryFilters) missionHistorySummary {
	filters.Route = strings.TrimSpace(filters.Route)
	filters.Status = strings.TrimSpace(filters.Status)
	filters.Query = strings.TrimSpace(filters.Query)
	if filters.Route == "" && filters.Status == "" && filters.Query == "" && !filters.PilotReadiness {
		return summary
	}
	filtered := make([]missionRouteHistoryItem, 0, len(summary.Routes))
	for _, item := range summary.Routes {
		if filters.Route != "" && !strings.EqualFold(item.Route, filters.Route) {
			continue
		}
		if filters.Status != "" && !strings.EqualFold(item.Status, filters.Status) {
			continue
		}
		if filters.Query != "" && !missionRouteHistoryItemMatchesQuery(item, filters.Query) {
			continue
		}
		if filters.PilotReadiness && !missionRouteHistoryItemMatchesPilotReadiness(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	summary.TotalRouteCount = len(summary.Routes)
	summary.RouteFilter = filters.Route
	summary.StatusFilter = filters.Status
	summary.QueryFilter = filters.Query
	summary.PilotReadinessFilter = filters.PilotReadiness
	summary.RouteCount = len(filtered)
	summary.Routes = filtered
	if len(filtered) == 0 {
		summary.LatestRoute = ""
		summary.ExactNextAction = "No Mission route-history entries matched the compact timeline filters"
		return summary
	}
	latest := filtered[len(filtered)-1]
	summary.LatestRoute = latest.Route
	summary.ExactNextAction = latest.ExactNextAction
	return summary
}

func missionRouteHistoryItemMatchesQuery(item missionRouteHistoryItem, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		item.Route,
		item.Reason,
		item.ExactNextAction,
		item.Status,
	}, " "))
	return strings.Contains(haystack, query)
}

func missionRouteHistoryItemMatchesPilotReadiness(item missionRouteHistoryItem) bool {
	haystack := strings.ToLower(strings.Join([]string{
		item.Route,
		item.Reason,
		item.ExactNextAction,
		item.Status,
	}, " "))
	for _, term := range []string{
		"pilot",
		"beta",
		"canary",
		"readiness",
		"approval inbox",
		"stop-rule",
		"incident",
	} {
		if strings.Contains(haystack, term) {
			return true
		}
	}
	return false
}

func missionHistoryFilterLabel(summary missionHistorySummary) string {
	parts := []string{}
	if summary.RouteFilter != "" {
		parts = append(parts, "route:"+summary.RouteFilter)
	}
	if summary.StatusFilter != "" {
		parts = append(parts, "status:"+summary.StatusFilter)
	}
	if summary.QueryFilter != "" {
		parts = append(parts, "query:"+summary.QueryFilter)
	}
	if summary.PilotReadinessFilter {
		parts = append(parts, "pilot_readiness:true")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}
