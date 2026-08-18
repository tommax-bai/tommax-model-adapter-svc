// Package conf 定义服务配置结构（docs/04 §1.8：所有配置项必须出现在这里）。
package conf

type Config struct {
	Server struct {
		GRPCAddr string `yaml:"grpcAddr"`
	} `yaml:"server"`
	Log struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"log"`
	Mock struct {
		LatencyMs int `yaml:"latencyMs"`
	} `yaml:"mock"`
	Job struct {
		TTLMinutes int `yaml:"ttlMinutes"`
	} `yaml:"job"`
}
