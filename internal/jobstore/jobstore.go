// Package jobstore 维护作业状态。Phase 1 为内存实现 + TTL 清理；
// 供应商真实接入后（回调跨实例）需换 Redis/PG 实现，接口保持不变。
package jobstore

import (
	"sync"
	"time"

	"github.com/tommax-bai/tommax-model-adapter-svc/internal/core"
)

type Store struct {
	mu       sync.RWMutex
	byID     map[string]*core.Job
	byTaskID map[string]string // task_id → job_id，幂等
	ttl      time.Duration
}

func New(ttl time.Duration) *Store {
	s := &Store{byID: make(map[string]*core.Job), byTaskID: make(map[string]string), ttl: ttl}
	go s.cleanupLoop()
	return s
}

// Put 登记作业；同一 task_id 已存在时返回已有作业（幂等）。
func (s *Store) Put(job *core.Job) (*core.Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byTaskID[job.TaskID]; ok {
		return s.byID[id], false
	}
	s.byID[job.ID] = job
	s.byTaskID[job.TaskID] = job.ID
	return job, true
}

func (s *Store) Get(jobID string) (*core.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.byID[jobID]
	return j, ok
}

// Update 供 provider 回写结果。
func (s *Store) Update(jobID string, result core.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.byID[jobID]; ok {
		j.Result = result
	}
}

// Snapshot 返回作业结果的拷贝，避免调用方读到并发修改中的切片。
func (s *Store) Snapshot(jobID string) (core.Result, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.byID[jobID]
	if !ok {
		return core.Result{}, false
	}
	r := j.Result
	r.Outputs = append([]core.Output(nil), j.Result.Outputs...)
	return r, true
}

func (s *Store) cleanupLoop() {
	t := time.NewTicker(time.Minute)
	for range t.C {
		cutoff := time.Now().Add(-s.ttl)
		s.mu.Lock()
		for id, j := range s.byID {
			if j.CreatedAt.Before(cutoff) {
				delete(s.byID, id)
				delete(s.byTaskID, j.TaskID)
			}
		}
		s.mu.Unlock()
	}
}
