package worker

type RoutingMode = string

const (
	None     RoutingMode = "none"
	Global   RoutingMode = "global"
	BypassCN RoutingMode = "bypass_cn"
)
