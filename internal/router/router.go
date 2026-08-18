// Package router 把 provider_model（"<provider>/<model>"）路由到已注册的 provider 实例。
// 熔断/限流/备用路由挂在这一层（Phase 1 暂未接入，接口位置已预留）。
package router

import (
	"fmt"
	"strings"

	"github.com/tommax-bai/tommax-model-adapter-svc/internal/core"
)

type Router struct {
	providers map[string]core.Provider
}

func New(providers ...core.Provider) *Router {
	m := make(map[string]core.Provider, len(providers))
	for _, p := range providers {
		m[p.Name()] = p
	}
	return &Router{providers: m}
}

// Resolve 拆解 provider_model 并返回 provider 与其内部模型名。
func (r *Router) Resolve(providerModel string) (core.Provider, string, error) {
	name, model, ok := strings.Cut(providerModel, "/")
	if !ok {
		return nil, "", fmt.Errorf("invalid provider_model %q, want <provider>/<model>", providerModel)
	}
	p, exists := r.providers[name]
	if !exists {
		return nil, "", fmt.Errorf("provider %q not registered", name)
	}
	return p, model, nil
}
