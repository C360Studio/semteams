package entityextraction

type gateThresholds struct {
	CoverageFloor                 float64 `json:"coverage_floor"`
	WrongConfidentCeiling         float64 `json:"wrong_confident_ceiling"`
	FalseMergeCeiling             float64 `json:"false_merge_ceiling"`
	ConfidenceThresholdMercury    float64 `json:"confidence_threshold_mercury"`
	ConfidenceThresholdSemselect  float64 `json:"confidence_threshold_semselect"`
	SupportThresholdYes           float64 `json:"support_threshold_yes"`
	SupportThresholdNo            float64 `json:"support_threshold_no"`
	ContradictionThresholdMercury float64 `json:"contradiction_threshold_mercury"`
	ContradictionThresholdSemsel  float64 `json:"contradiction_threshold_semselect"`
}

type catalogEntity struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
}

type caseExpect struct {
	Type        string `json:"type"`
	Link        string `json:"link"`
	Mention     bool   `json:"mention"`
	Support     bool   `json:"support"`
	SupportNote string `json:"support_note,omitempty"`
}

type corpusCase struct {
	ID            string     `json:"id"`
	ArtifactID    string     `json:"artifact_id"`
	Band          string     `json:"band"`
	BandName      string     `json:"band_name"`
	Name          string     `json:"name"`
	Role          string     `json:"role"`
	Quote         string     `json:"quote"`
	Locator       string     `json:"locator"`
	Channel       string     `json:"channel"`
	SourceClass   string     `json:"source_class"`
	ToolResultRef string     `json:"tool_result_ref"`
	Expect        caseExpect `json:"expect"`
}

type corpus struct {
	Schema         string          `json:"schema"`
	Version        string          `json:"version"`
	CreatedAt      string          `json:"created_at"`
	Description    string          `json:"description"`
	GateThresholds gateThresholds  `json:"gate_thresholds"`
	SeedCatalog    []catalogEntity `json:"seed_catalog"`
	Cases          []corpusCase    `json:"cases"`
}

type evalRow struct {
	Case                 string   `json:"case"`
	Band                 string   `json:"band"`
	Tier                 string   `json:"tier"`
	Type                 string   `json:"type"`
	TypeRaw              string   `json:"type_raw,omitempty"`
	TypeConf             *float64 `json:"type_conf"`
	Link                 string   `json:"link"`
	LinkRaw              string   `json:"link_raw,omitempty"`
	LinkConf             *float64 `json:"link_conf"`
	Mention              *bool    `json:"mention,omitempty"`
	PSupport             *float64 `json:"p_support,omitempty"`
	ContradictionFlagged *bool    `json:"contradiction_flagged,omitempty"`
	SupportMention       any      `json:"support_mention"`
	SupportStrict        any      `json:"support_strict"`
	SupportContradiction any      `json:"support_contradiction,omitempty"`
	T1Candidates         []string `json:"t1_candidates,omitempty"`
	InputTokens          int      `json:"input_tokens"`
	LatencyMS            int      `json:"latency_ms"`
	Cached               *bool    `json:"cached,omitempty"`
}

type bandSummary struct {
	Count                 int     `json:"count"`
	TypeCoverage          float64 `json:"type_coverage"`
	TypeOK                string  `json:"type_ok"`
	TypeWrongConfident    float64 `json:"type_wrong_confident"`
	LinkCoverage          float64 `json:"link_coverage"`
	LinkOK                string  `json:"link_ok"`
	FalseMergeRate        float64 `json:"false_merge_rate"`
	SupportCoverage       float64 `json:"support_coverage"`
	SupportOK             string  `json:"support_ok"`
	SupportWrongConfident float64 `json:"support_wrong_confident"`
	InputTokens           int     `json:"input_tokens"`
	MedianMS              int     `json:"median_ms"`
}

type algoEvalOutput struct {
	Arm                            string                 `json:"arm"`
	GateThresholds                 gateThresholds         `json:"gate_thresholds"`
	WarmCatalogSize                int                    `json:"warm_catalog_size"`
	PortfolioCatalogSize           int                    `json:"portfolio_catalog_size"`
	EmptyCatalogSize               int                    `json:"empty_catalog_size"`
	WarmTypeLinkResidualCalls      string                 `json:"warm_type_link_residual_calls"`
	PortfolioTypeLinkResidualCalls string                 `json:"portfolio_type_link_residual_calls"`
	EmptyTypeLinkResidualCalls     string                 `json:"empty_type_link_residual_calls"`
	Rows                           []evalRow              `json:"rows"`
	RowsPortfolioCatalog           []evalRow              `json:"rows_portfolio_catalog"`
	RowsEmptyCatalog               []evalRow              `json:"rows_empty_catalog"`
	ByBandMentionCheck             map[string]bandSummary `json:"by_band_mention_check"`
	ByBandMentionAsRoleSupport     map[string]bandSummary `json:"by_band_mention_as_role_support"`
	ByBandStrictRoleSupport        map[string]bandSummary `json:"by_band_strict_role_support"`
	ByBandPortfolioCatalog         map[string]bandSummary `json:"by_band_portfolio_catalog"`
	ByBandEmptyCatalog             map[string]bandSummary `json:"by_band_empty_catalog"`
}

type modelEvalOutput struct {
	Arm                    string                 `json:"arm"`
	Order                  string                 `json:"order"`
	GateThresholds         gateThresholds         `json:"gate_thresholds"`
	ElapsedS               float64                `json:"elapsed_s"`
	Rows                   []evalRow              `json:"rows"`
	ByBand                 map[string]bandSummary `json:"by_band"`
	ByBandContradictionFlg map[string]bandSummary `json:"by_band_contradiction_flag"`
}

type hybridEvalOutput struct {
	Arm                              string                 `json:"arm"`
	GateThresholds                   gateThresholds         `json:"gate_thresholds"`
	RoutingPolicy                    string                 `json:"routing_policy"`
	TypeLinkOnlyModelCalls           string                 `json:"type_link_only_model_calls"`
	PortfolioCatalogModelCalls       string                 `json:"portfolio_catalog_model_calls"`
	EmptyCatalogModelCalls           string                 `json:"empty_catalog_model_calls"`
	CueRouterModelCalls              string                 `json:"cue_router_model_calls"`
	StrictSupportModelCalls          string                 `json:"strict_support_model_calls"`
	TotalInputTokensStrictSupport    int                    `json:"total_input_tokens_strict_support"`
	TotalInputTokensCueRouter        int                    `json:"total_input_tokens_cue_router"`
	ByBand                           map[string]bandSummary `json:"by_band"`
	ByBandContradictionFlag          map[string]bandSummary `json:"by_band_contradiction_flag"`
	ByBandCueRouterStrictSupport     map[string]bandSummary `json:"by_band_cue_router_strict_support"`
	ByBandCueRouterContradictMention map[string]bandSummary `json:"by_band_cue_router_contradiction_and_mention"`
	ByBandPortfolioCatalog           map[string]bandSummary `json:"by_band_portfolio_catalog"`
	ByBandEmptyCatalog               map[string]bandSummary `json:"by_band_empty_catalog"`
	Rows                             []evalRow              `json:"rows"`
	RowsCueRouter                    []evalRow              `json:"rows_cue_router"`
	RowsPortfolioCatalog             []evalRow              `json:"rows_portfolio_catalog"`
	RowsEmptyCatalog                 []evalRow              `json:"rows_empty_catalog"`
}

type agreeEvalOutput struct {
	Arm                        string                 `json:"arm"`
	GateThresholds             gateThresholds         `json:"gate_thresholds"`
	RoutingPolicy              string                 `json:"routing_policy"`
	ResidualCasesQueried       string                 `json:"residual_cases_queried"`
	TotalModelRequests         string                 `json:"total_model_requests"`
	TotalInputTokens           int                    `json:"total_input_tokens"`
	ByBandStrictBothAboveFloor map[string]bandSummary `json:"by_band"`
	ByBandMeanProbAgreement    map[string]bandSummary `json:"by_band_mean_prob_agreement"`
	ByBandContradictionFlag    map[string]bandSummary `json:"by_band_contradiction_flag"`
	Rows                       []evalRow              `json:"rows"`
}
