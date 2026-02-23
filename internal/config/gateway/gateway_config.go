package gateway

// GatewayConfig holds gateway server settings.
type GatewayConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func DefaultGatewayConfig() GatewayConfig {
	return GatewayConfig{Host: "0.0.0.0", Port: 18790}
}
