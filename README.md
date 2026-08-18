# tommax-model-adapter-svc

模型供应商适配层：统一 Submit/Query/Cancel gRPC 协议，一供应商一插件（`internal/provider/<vendor>`），密钥/限流/熔断/成本归此层。不含业务语义（docs/02 §3.4）。

## 本地启动
```bash
make run   # gRPC :9101
```

## 新接一个供应商
1. 新建 `internal/provider/<vendor>`，实现 `core.Provider`（参考 `provider/mock`）；
2. 在 `cmd/server/main.go` 的 router.New(...) 注册；
3. 在 generation-svc 的 `configs/model_catalog.yaml` 增加 `providerModel: <vendor>/<model>` 条目。
不改任何既有代码（开闭原则，docs/03 §1.4）。

## 例外登记
| 例外 | 原因 | 回收条件 |
|---|---|---|
| jobstore 为内存实现 | mock 单实例够用 | 接入第一个真实供应商（回调跨实例）时换 Redis/PG 实现 |
| 熔断/限流未挂 router | Phase 1 无真实供应商 | 首个真实 provider 接入时启用 gobreaker + rate |

负责人：TBD
