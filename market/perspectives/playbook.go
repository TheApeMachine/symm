package perspectives

// PlaybookName identifies which registered perspective authorized a position.
type PlaybookName string

const (
	PlaybookTrend    PlaybookName = "trend"
	PlaybookDrive    PlaybookName = "drive"
	PlaybookLeadLag  PlaybookName = "leadlag"
	PlaybookScarcity PlaybookName = "scarcity"
	PlaybookPump     PlaybookName = "pump"
	PlaybookUniversal PlaybookName = "universal"
)

/*
EntryPolicy selects which global entry gates apply. Pump and drive are allowed to
operate during systemic slumps; trend-like playbooks require breadth or deny slump.
*/
type EntryPolicy uint8

const (
	EntryPolicyStandard EntryPolicy = iota
	EntryPolicyDrive
	EntryPolicyPump
)
