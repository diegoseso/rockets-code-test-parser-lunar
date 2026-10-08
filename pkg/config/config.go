package config

type Values struct {
	GrpcServer string `mapstructure:"grpc_server"`
	HTTP       string `mapstructure:"http"`
	AuditLog   bool   `mapstructure:"audit_log"`
}
