package controller

var NFCycleCosts = map[string]int{
	"fc":          100,
	"nat":         250,
	"ratelimiter": 50,
	"filter":      50,
	"chacha":      2000,
	"aesenc":      40000,
	"aesdec":      60000,
	"acl":         250,
	"none":        50,
}
