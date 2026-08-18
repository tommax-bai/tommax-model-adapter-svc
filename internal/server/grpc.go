// Package server 实现 proto InferenceService 与 core 协议的转换（接口层，不含业务判断）。
package server

import (
	"context"
	"log/slog"
	"time"

	modeladapterv1 "github.com/tommax-bai/tommax-proto/gen/go/tommax/modeladapter/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tommax-bai/tommax-go-kit/idgen"
	"github.com/tommax-bai/tommax-model-adapter-svc/internal/core"
	"github.com/tommax-bai/tommax-model-adapter-svc/internal/jobstore"
	"github.com/tommax-bai/tommax-model-adapter-svc/internal/router"
)

type InferenceServer struct {
	modeladapterv1.UnimplementedInferenceServiceServer
	router *router.Router
	jobs   *jobstore.Store
}

func NewInferenceServer(r *router.Router, jobs *jobstore.Store) *InferenceServer {
	return &InferenceServer{router: r, jobs: jobs}
}

var capFromProto = map[modeladapterv1.Capability]core.Capability{
	modeladapterv1.Capability_CAPABILITY_IMAGE_TEXT2IMG:   core.CapImageText2Img,
	modeladapterv1.Capability_CAPABILITY_IMAGE_REF2IMG:    core.CapImageRef2Img,
	modeladapterv1.Capability_CAPABILITY_VIDEO_TEXT2VIDEO: core.CapVideoText2Vid,
	modeladapterv1.Capability_CAPABILITY_VIDEO_IMG2VIDEO:  core.CapVideoImg2Vid,
}

func (s *InferenceServer) Submit(ctx context.Context, req *modeladapterv1.SubmitRequest) (*modeladapterv1.SubmitResponse, error) {
	if req.GetTaskId() == "" || req.GetProviderModel() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id and provider_model are required")
	}
	cap, ok := capFromProto[req.GetCapability()]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported capability %v", req.GetCapability())
	}
	provider, model, err := s.router.Resolve(req.GetProviderModel())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	job := &core.Job{
		ID:       idgen.NextString(),
		TaskID:   req.GetTaskId(),
		Provider: provider.Name(),
		Request: core.Request{
			TaskID:        req.GetTaskId(),
			ProviderModel: req.GetProviderModel(),
			Model:         model,
			Capability:    cap,
			Prompt:        req.GetPrompt(),
			RefURLs:       req.GetRefUrls(),
			Params:        req.GetParams(),
		},
		Result: core.Result{Status: core.StatusRunning},
	}
	job.CreatedAt = time.Now()

	stored, isNew := s.jobs.Put(job)
	if isNew {
		if err := provider.Submit(ctx, stored, s.jobs.Update); err != nil {
			s.jobs.Update(stored.ID, core.Result{Status: core.StatusFailedRetryable, ErrorMsg: err.Error()})
			slog.Error("provider submit failed", "provider", provider.Name(), "taskId", req.GetTaskId(), "err", err)
		}
	}
	return &modeladapterv1.SubmitResponse{JobId: stored.ID}, nil
}

func (s *InferenceServer) Query(_ context.Context, req *modeladapterv1.QueryRequest) (*modeladapterv1.QueryResponse, error) {
	result, ok := s.jobs.Snapshot(req.GetJobId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "job %s not found", req.GetJobId())
	}
	resp := &modeladapterv1.QueryResponse{
		Status:       statusToProto(result.Status),
		Progress:     int32(result.Progress),
		ErrorMessage: result.ErrorMsg,
	}
	for _, o := range result.Outputs {
		resp.Outputs = append(resp.Outputs, &modeladapterv1.Output{
			Url:      o.URL,
			Data:     o.Data,
			MimeType: o.MimeType,
			Width:    int32(o.Width),
			Height:   int32(o.Height),
		})
	}
	return resp, nil
}

func (s *InferenceServer) Cancel(ctx context.Context, req *modeladapterv1.CancelRequest) (*modeladapterv1.CancelResponse, error) {
	job, ok := s.jobs.Get(req.GetJobId())
	if !ok {
		return &modeladapterv1.CancelResponse{Canceled: false}, nil
	}
	provider, _, err := s.router.Resolve(job.Request.ProviderModel)
	if err != nil {
		return &modeladapterv1.CancelResponse{Canceled: false}, nil
	}
	_ = provider.Cancel(ctx, job)
	return &modeladapterv1.CancelResponse{Canceled: true}, nil
}

func statusToProto(st core.JobStatus) modeladapterv1.JobStatus {
	switch st {
	case core.StatusRunning:
		return modeladapterv1.JobStatus_JOB_STATUS_RUNNING
	case core.StatusSucceeded:
		return modeladapterv1.JobStatus_JOB_STATUS_SUCCEEDED
	case core.StatusFailedRetryable:
		return modeladapterv1.JobStatus_JOB_STATUS_FAILED_RETRYABLE
	case core.StatusFailedPermanent:
		return modeladapterv1.JobStatus_JOB_STATUS_FAILED_PERMANENT
	case core.StatusContentBlocked:
		return modeladapterv1.JobStatus_JOB_STATUS_CONTENT_BLOCKED
	default:
		return modeladapterv1.JobStatus_JOB_STATUS_UNSPECIFIED
	}
}
