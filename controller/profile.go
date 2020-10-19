package controller

const (
	// The cycle cost of an empty queue module.
	DEFAULT_CYCLE_COST = 50
)

// Currently, we've supported these NFs in the NF runtime.
// [fc, nat, ratelimiter, filter, chacha, aesenc, aesdec, acl]
// Others are implemented as dummy NFs.
// TODO(Jianfeng): we need to migrate more NFs from Lemur.
var NFCycleCosts = map[string]int{
	"acl":         985,
	"aesenc":      44000,
	"aesdec":      63000,
	"evpaescbc":   9100,
	"evpaescbcde": 2800,
	"chacha":      6800,
	"fc":          100,
	"filter":      50,
	"hashlb":      560,
	"nat":         1500,
	"ratelimiter": 50,
	"updatettl":   60,
	"urlfilter":   6900,
	"vlanpush":    290,
	"vlanpop":     230,
	"none":        DEFAULT_CYCLE_COST,
}
