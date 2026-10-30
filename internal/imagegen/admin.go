package imagegen

import sql "github.com/FloatTech/sqlite"

type BudgetStatus struct {
	Month        string `json:"month"`
	Timezone     string `json:"timezone"`
	ChargedUSD   string `json:"chargedUSD"`
	RemainingUSD string `json:"remainingUSD"`
	UncertainUSD string `json:"uncertainUSD"`
	Requests     int    `json:"requests"`
	Uncertain    int    `json:"uncertain"`
	Stopped      bool   `json:"stopped"`
}
type GenerationOptions struct {
	ReferenceReserveUSD string `json:"referenceReserveUSD"`
	Background          string `json:"background"`
	Format              string `json:"format"`
	Compression         int    `json:"compression"`
	Count               int    `json:"count"`
	Moderation          string `json:"moderation"`
	Stream              bool   `json:"stream"`
	PartialImages       int    `json:"partialImages"`
	SafetyIdentifier    string `json:"safetyIdentifier"`
	ProxyURL            string `json:"proxyURL"`
	TimeoutSeconds      int    `json:"timeoutSeconds"`
}
type Snapshot struct {
	Options          GenerationOptions `json:"options"`
	Revision         string            `json:"revision"`
	Mode             string            `json:"mode"`
	URL              string            `json:"url"`
	Model            string            `json:"model"`
	KeyConfigured    bool              `json:"keyConfigured"`
	Quality          string            `json:"quality"`
	Size             string            `json:"size"`
	MonthlyBudgetUSD string            `json:"monthlyBudgetUSD"`
	CustomCostUSD    string            `json:"customCostUSD"`
	Budget           BudgetStatus      `json:"budget"`
	PricingNote      string            `json:"pricingNote"`
	PricingReady     bool              `json:"pricingReady"`
}
type Update struct {
	Options          *GenerationOptions `json:"options"`
	Revision         string             `json:"revision"`
	Mode             string             `json:"mode"`
	URL              string             `json:"url"`
	Model            string             `json:"model"`
	Quality          string             `json:"quality"`
	Size             string             `json:"size"`
	MonthlyBudgetUSD string             `json:"monthlyBudgetUSD"`
	CustomCostUSD    string             `json:"customCostUSD"`
	Key              struct {
		Action  string `json:"action"`
		Value   string `json:"value"`
		Confirm bool   `json:"confirm"`
	} `json:"key"`
}
type Backup func(*sql.Sqlite) error
type Admin func(*Update, Backup) (Snapshot, error)

var RuntimeAdmin Admin
