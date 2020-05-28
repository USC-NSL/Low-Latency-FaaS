package controller

const (
	// The cycle cost of an empty queue module.
	DEFAULT_CYCLE_COST = 50
)

var NFCycleCosts = map[string]int{
	"fc":          100,
	"nat":         250,
	"ratelimiter": 50,
	"filter":      50,
	"chacha":      2000,
	"aesenc":      40000,
	"aesdec":      60000,
	"acl":         250,
	"none":        DEFAULT_CYCLE_COST,
}
