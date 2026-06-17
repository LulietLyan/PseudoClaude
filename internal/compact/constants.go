package compact

const (
	SingleToolResultLimitBytes      = 50000
	ToolRoundAggregateLimitBytes    = 200000
	SummaryReserveTokens            = 20000
	AutoSafetyMarginTokens          = 13000
	ManualSafetyMarginTokens        = 3000
	RecentKeepTokens                = 10000
	RecentKeepMessages              = 5
	AutoFailureLimit                = 3
	EstimateCharsPerToken           = 3.5
	PreviewHeadBytes                = 2048
	PreviewHeadLines                = 20
	SummaryRetryLimit               = 3
	DefaultAnthropicContextWindow   = 200000
	DefaultOpenAIContextWindow      = 128000
	defaultFallbackContextWindow    = 128000
	summaryPromptTooLargeDropGroups = 1
)
