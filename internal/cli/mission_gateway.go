package cli

import (
	"flag"
	"fmt"
	"sort"
	"strings"
)

func (a App) missionGateway(args []string) int {
	var readbackPath string
	var jsonOut bool
	fs := flag.NewFlagSet("mission gateway", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&readbackPath, "readback", "", "path to AO Mission gateway replay or ledger JSON")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(readbackPath) == "" {
		fmt.Fprintln(a.Stderr, "ao-command mission gateway: --readback is required")
		return 2
	}
	summary, err := readMissionGatewayReadback(readbackPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "ao-command mission gateway: %v\n", err)
		return 1
	}
	if jsonOut {
		return a.writeJSON(summary)
	}
	fmt.Fprintf(a.Stdout, "ao_command_mission_gateway=%s\n", summary.Status)
	fmt.Fprintf(a.Stdout, "mission_id=%s\n", summary.MissionID)
	fmt.Fprintf(a.Stdout, "gateway_count=%d\n", summary.GatewayCount)
	fmt.Fprintf(a.Stdout, "total=%d\n", summary.Total)
	fmt.Fprintf(a.Stdout, "intent_recorded=%d\n", summary.IntentRecorded)
	fmt.Fprintf(a.Stdout, "denied=%d\n", summary.Denied)
	fmt.Fprintf(a.Stdout, "invalid=%d\n", summary.Invalid)
	fmt.Fprintf(a.Stdout, "operator_mode=%s\n", summary.OperatorMode)
	fmt.Fprintf(a.Stdout, "safe_to_execute=%t\n", summary.SafeToExecute)
	fmt.Fprintf(a.Stdout, "executes_work=%t\n", summary.ExecutesWork)
	fmt.Fprintf(a.Stdout, "approves_work=%t\n", summary.ApprovesWork)
	fmt.Fprintf(a.Stdout, "mutates_repositories=%t\n", summary.MutatesRepositories)
	fmt.Fprintf(a.Stdout, "exact_next_action=%s\n", summary.ExactNextAction)
	return 0
}

type missionGatewaySummary struct {
	CommandSchemaVersion string   `json:"command_schema_version"`
	Schema               string   `json:"schema"`
	MissionID            string   `json:"mission_id"`
	Status               string   `json:"status"`
	OperatorMode         string   `json:"operator_mode"`
	GatewayCount         int      `json:"gateway_count"`
	Gateways             []string `json:"gateways"`
	Total                int      `json:"total"`
	IntentRecorded       int      `json:"intent_recorded"`
	Denied               int      `json:"denied"`
	Invalid              int      `json:"invalid"`
	SafeToExecute        bool     `json:"safe_to_execute"`
	ExecutesWork         bool     `json:"executes_work"`
	ApprovesWork         bool     `json:"approves_work"`
	MutatesRepositories  bool     `json:"mutates_repositories"`
	ExactNextAction      string   `json:"exact_next_action"`
}

func readMissionGatewayReadback(path string) (missionGatewaySummary, error) {
	var input struct {
		Schema              string `json:"schema"`
		MissionID           string `json:"mission_id"`
		Status              string `json:"status"`
		Gateway             string `json:"gateway"`
		TelegramReadbacks   int    `json:"telegram_readbacks"`
		A2AReadbacks        int    `json:"a2a_readbacks"`
		SchedulerReadbacks  int    `json:"scheduler_readbacks"`
		TotalIntents        int    `json:"total_intents"`
		Total               int    `json:"total"`
		IntentRecorded      int    `json:"intent_recorded"`
		Denied              int    `json:"denied"`
		Invalid             int    `json:"invalid"`
		MutationAuthority   bool   `json:"mutation_authority"`
		ExecutesWork        bool   `json:"executes_work"`
		ApprovesWork        bool   `json:"approves_work"`
		MutatesRepositories bool   `json:"mutates_repositories"`
		Intents             []struct {
			Gateway           string `json:"gateway"`
			Status            string `json:"status"`
			MutationAuthority bool   `json:"mutation_authority"`
			ExecutesWork      bool   `json:"executes_work"`
			ApprovesWork      bool   `json:"approves_work"`
		} `json:"intents"`
	}
	if err := readJSONFile(path, &input); err != nil {
		return missionGatewaySummary{}, err
	}
	switch input.Schema {
	case "ao.mission.gateway-intent-ledger.v0.1", "ao.mission.telegram-replay-readback.v0.1", "ao.mission.telegram-update-replay-readback.v0.1", "ao.mission.a2a-http-replay-readback.v0.1", "ao.mission.gateway-replay-bundle-readback.v0.1":
	default:
		return missionGatewaySummary{}, fmt.Errorf("unsupported mission gateway schema %q", input.Schema)
	}
	if input.MutationAuthority || input.ExecutesWork || input.ApprovesWork || input.MutatesRepositories {
		return missionGatewaySummary{}, fmt.Errorf("mission gateway readback must not claim execution, approval, or repository mutation authority")
	}
	gatewaySet := map[string]bool{}
	if input.Gateway != "" {
		gatewaySet[input.Gateway] = true
	}
	if input.TelegramReadbacks > 0 {
		gatewaySet["telegram"] = true
	}
	if input.A2AReadbacks > 0 {
		gatewaySet["a2a"] = true
	}
	if input.SchedulerReadbacks > 0 {
		gatewaySet["scheduler"] = true
	}
	for _, intent := range input.Intents {
		if intent.MutationAuthority || intent.ExecutesWork || intent.ApprovesWork {
			return missionGatewaySummary{}, fmt.Errorf("mission gateway intent must not claim execution or approval authority")
		}
		if intent.Gateway != "" {
			gatewaySet[intent.Gateway] = true
		}
	}
	gateways := make([]string, 0, len(gatewaySet))
	for gateway := range gatewaySet {
		gateways = append(gateways, gateway)
	}
	sort.Strings(gateways)
	total := input.Total
	intentRecorded := input.IntentRecorded
	if input.Schema == "ao.mission.gateway-replay-bundle-readback.v0.1" {
		total = input.TotalIntents
		intentRecorded = input.TotalIntents
	}
	return missionGatewaySummary{
		CommandSchemaVersion: commandSchemaVersion,
		Schema:               "ao.command.mission-gateway.v0.1",
		MissionID:            input.MissionID,
		Status:               input.Status,
		OperatorMode:         operatorMode,
		GatewayCount:         len(gateways),
		Gateways:             gateways,
		Total:                total,
		IntentRecorded:       intentRecorded,
		Denied:               input.Denied,
		Invalid:              input.Invalid,
		SafeToExecute:        false,
		ExecutesWork:         false,
		ApprovesWork:         false,
		MutatesRepositories:  false,
		ExactNextAction:      "inspect gateway intent ledger; continue through AO Mission routing, not Telegram or A2A authority",
	}, nil
}
