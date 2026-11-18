// Package httpapi serves the OpenAPI contract in api/openapi.yaml.
//
// api.gen.go is generated (`task api:gen`); this file implements its
// StrictServerInterface by translating between wire types and the service.
//
//go:generate go tool oapi-codegen -config ../../api/oapi-codegen.yaml ../../api/openapi.yaml
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/service"
)

type Server struct {
	svc    *service.Service
	log    *slog.Logger
	limits limiters
}

var _ StrictServerInterface = (*Server)(nil)

// Mount registers the API on r, every route behind bearer authentication.
func Mount(r chi.Router, svc *service.Service, log *slog.Logger, limits Limits) http.Handler {
	s := &Server{svc: svc, log: log, limits: newLimiters(limits)}
	strict := NewStrictHandlerWithOptions(s, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("request failed", slog.String("path", r.URL.Path), slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, "internal error")
		},
	})
	return HandlerWithOptions(strict, ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []MiddlewareFunc{s.authenticate},
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, err.Error())
		},
	})
}

type userKey struct{}

// authenticate runs before the request body is decoded, so an unauthenticated
// caller learns nothing about what a valid request looks like.
//
// Only a request that fails is counted against its address. Past the limit
// the answer is 429 instead of 401, so a client guessing tokens, or one stuck
// in a loop with a revoked one, is told to stop rather than told "no" again.
func (s *Server) authenticate(next http.Handler) http.Handler {
	refuse := func(w http.ResponseWriter, r *http.Request, msg string) {
		if ok, wait := s.limits.failed.Allow(clientAddress(r)); !ok {
			writeTooManyRequests(w, wait)
			return
		}
		writeError(w, http.StatusUnauthorized, msg)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			refuse(w, r, "missing bearer token")
			return
		}
		userID, err := s.svc.Authenticate(r.Context(), strings.TrimSpace(token))
		if errors.Is(err, service.ErrUnauthenticated) {
			refuse(w, r, "unknown token")
			return
		}
		if err != nil {
			s.log.Error("authenticate", slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, userID)))
	})
}

func userID(ctx context.Context) uuid.UUID {
	// Present on every route: authenticate wraps them all, and a handler
	// reached without it is a wiring bug worth crashing the request over.
	return ctx.Value(userKey{}).(uuid.UUID)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Error{Message: msg})
}

func (s *Server) ListPlatforms(ctx context.Context, _ ListPlatformsRequestObject) (ListPlatformsResponseObject, error) {
	out := ListPlatforms200JSONResponse{}
	for _, p := range s.svc.Platforms() {
		r := p.Rules()
		connect := Webhook
		if s.svc.ConnectsByOAuth(p.Platform()) {
			connect = Oauth
		}
		out = append(out, PlatformInfo{
			Platform: Platform(p.Platform()),
			Connect:  connect,
			Rules: Rules{
				MaxTextLength:      r.MaxTextLength,
				TextCounting:       textCounting(r.TextCounting),
				MaxLinks:           r.MaxLinks,
				MaxMedia:           r.MaxMedia,
				MaxMediaBytes:      r.MaxMediaBytes,
				MaxTotalMediaBytes: r.MaxTotalMediaBytes,
				MaxVideoBytes:      r.MaxVideoBytes,
				VideoAlone:         r.VideoAlone,
				MinVideoMs:         int(r.MinVideoLength.Milliseconds()),
				MaxVideoMs:         int(r.MaxVideoLength.Milliseconds()),
				MinVideoAspect:     r.MinVideoAspect,
				MaxVideoAspect:     r.MaxVideoAspect,
				MaxVideoWidth:      r.MaxVideoWidth,
				MediaTypes:         r.MediaTypes,
				RequiresText:       r.RequiresText,
				RequiresMedia:      r.RequiresMedia,
				Options:            postOptions(r.Options),
				MaxCommentLength:   r.MaxCommentLength,
			},
		})
	}
	return out, nil
}

func (s *Server) ListAccounts(ctx context.Context, _ ListAccountsRequestObject) (ListAccountsResponseObject, error) {
	rows, err := s.svc.ListAccounts(ctx, userID(ctx))
	if err != nil {
		return nil, err
	}
	out := ListAccounts200JSONResponse{}
	for _, a := range rows {
		out = append(out, toAccount(a))
	}
	return out, nil
}

func (s *Server) ConnectDiscord(ctx context.Context, req ConnectDiscordRequestObject) (ConnectDiscordResponseObject, error) {
	acct, err := s.svc.ConnectDiscord(ctx, userID(ctx), req.Body.WebhookUrl, req.Body.DisplayName)
	if msg, ok := badRequest(err); ok {
		return ConnectDiscord400JSONResponse{BadRequestJSONResponse{Message: msg}}, nil
	}
	if err != nil {
		return nil, err
	}
	return ConnectDiscord201JSONResponse(toAccount(acct)), nil
}

func (s *Server) StartConnect(ctx context.Context, req StartConnectRequestObject) (StartConnectResponseObject, error) {
	if !req.Body.Client.Valid() {
		return StartConnect400JSONResponse{BadRequestJSONResponse{Message: "client must be web or ios"}}, nil
	}
	url, err := s.svc.StartConnect(ctx, userID(ctx), publisher.Platform(req.Platform), service.ConnectClient(req.Body.Client))
	if errors.Is(err, service.ErrNotFound) {
		return StartConnect404JSONResponse{NotFoundJSONResponse{Message: "this server does not connect " + string(req.Platform) + " accounts"}}, nil
	}
	if err != nil {
		return nil, err
	}
	return StartConnect200JSONResponse{AuthorizeUrl: url}, nil
}

func (s *Server) UpdateAccount(ctx context.Context, req UpdateAccountRequestObject) (UpdateAccountResponseObject, error) {
	acct, err := s.svc.SetAccountBrand(ctx, userID(ctx), req.AccountId, req.Body.Brand)
	if msg, ok := badRequest(err); ok {
		return UpdateAccount400JSONResponse{BadRequestJSONResponse{Message: msg}}, nil
	}
	if errors.Is(err, service.ErrNotFound) {
		return UpdateAccount404JSONResponse{NotFoundJSONResponse{Message: "no such account"}}, nil
	}
	if err != nil {
		return nil, err
	}
	return UpdateAccount200JSONResponse(toAccount(acct)), nil
}

func (s *Server) DeleteAccount(ctx context.Context, req DeleteAccountRequestObject) (DeleteAccountResponseObject, error) {
	err := s.svc.DeleteAccount(ctx, userID(ctx), req.AccountId)
	if errors.Is(err, service.ErrNotFound) {
		return DeleteAccount404JSONResponse{NotFoundJSONResponse{Message: "no such account"}}, nil
	}
	if err != nil {
		return nil, err
	}
	return DeleteAccount204Response{}, nil
}

func (s *Server) CreateUpload(ctx context.Context, req CreateUploadRequestObject) (CreateUploadResponseObject, error) {
	if ok, wait := s.limits.uploads.Allow(userID(ctx).String()); !ok {
		return CreateUpload429JSONResponse{tooManyRequests(wait)}, nil
	}
	up, err := s.svc.ReserveUpload(ctx, userID(ctx), req.Body.ContentType, req.Body.SizeBytes, service.VideoFacts{
		DurationMS: orZero(req.Body.DurationMs),
		Width:      orZero(req.Body.Width),
		Height:     orZero(req.Body.Height),
	})
	if msg, ok := badRequest(err); ok {
		return CreateUpload400JSONResponse{BadRequestJSONResponse{Message: msg}}, nil
	}
	if err != nil {
		return nil, err
	}
	return CreateUpload201JSONResponse{
		MediaId:   up.MediaID,
		UploadUrl: up.URL,
		Method:    PUT,
		Headers:   map[string]string{"Content-Type": up.ContentType},
		ExpiresAt: up.ExpiresAt,
	}, nil
}

func (s *Server) ListPosts(ctx context.Context, req ListPostsRequestObject) (ListPostsResponseObject, error) {
	limit := 20
	if req.Params.Limit != nil {
		limit = min(max(*req.Params.Limit, 1), 100)
	}
	views, err := s.svc.ListPosts(ctx, userID(ctx), limit)
	if err != nil {
		return nil, err
	}
	out := ListPosts200JSONResponse{}
	for _, v := range views {
		out = append(out, toPost(v))
	}
	return out, nil
}

func (s *Server) CreatePost(ctx context.Context, req CreatePostRequestObject) (CreatePostResponseObject, error) {
	// Before anything is read or stored: a refused request must cost nothing.
	// A repeat with an Idempotency-Key is counted too, since it is the cheap
	// way to hammer this route.
	if ok, wait := s.limits.posts.Allow(userID(ctx).String()); !ok {
		return CreatePost429JSONResponse{tooManyRequests(wait)}, nil
	}
	in := service.CreatePostInput{
		AccountIDs:     req.Body.AccountIds,
		ScheduledAt:    req.Body.ScheduledAt,
		IdempotencyKey: req.Params.IdempotencyKey,
	}
	if req.Body.Text != nil {
		in.Text = *req.Body.Text
	}
	if req.Body.FirstComment != nil {
		in.FirstComment = *req.Body.FirstComment
	}
	if req.Body.MediaIds != nil {
		in.MediaIDs = *req.Body.MediaIds
	}
	if req.Body.Options != nil {
		in.Options = map[uuid.UUID]map[string]string{}
		for raw, values := range *req.Body.Options {
			id, err := uuid.Parse(raw)
			if err != nil {
				return CreatePost400JSONResponse{BadRequestJSONResponse{Message: "options must be keyed by account id"}}, nil
			}
			in.Options[id] = values
		}
	}
	if req.Body.Assisted != nil {
		for _, p := range *req.Body.Assisted {
			in.Assisted = append(in.Assisted, string(p))
		}
	}

	view, err := s.svc.CreatePost(ctx, userID(ctx), in)
	var ve *service.ValidationError
	if errors.As(err, &ve) {
		out := CreatePost422JSONResponse{Message: "the post would be rejected", Issues: []Issue{}}
		for _, i := range ve.Issues {
			issue := Issue{AccountId: i.AccountID, Field: i.Field, Message: i.Message}
			if i.Platform != "" {
				issue.Platform = new(Platform(i.Platform))
			}
			out.Issues = append(out.Issues, issue)
		}
		return out, nil
	}
	if msg, ok := badRequest(err); ok {
		return CreatePost400JSONResponse{BadRequestJSONResponse{Message: msg}}, nil
	}
	if err != nil {
		return nil, err
	}
	return CreatePost201JSONResponse(toPost(view)), nil
}

func (s *Server) CompleteHandoff(ctx context.Context, req CompleteHandoffRequestObject) (CompleteHandoffResponseObject, error) {
	view, err := s.svc.CompleteHandoff(ctx, userID(ctx), req.TargetId, req.Body.Sent)
	switch {
	case errors.Is(err, service.ErrNotFound):
		return CompleteHandoff404JSONResponse{NotFoundJSONResponse{Message: "no such target"}}, nil
	case errors.Is(err, service.ErrConflict):
		return CompleteHandoff409JSONResponse{Message: "this target is not waiting for a handoff"}, nil
	case err != nil:
		return nil, err
	}
	return CompleteHandoff200JSONResponse(toPost(view)), nil
}

func (s *Server) RetryTarget(ctx context.Context, req RetryTargetRequestObject) (RetryTargetResponseObject, error) {
	view, err := s.svc.RetryTarget(ctx, userID(ctx), req.TargetId)
	var conflict *service.Conflict
	switch {
	case errors.Is(err, service.ErrNotFound):
		return RetryTarget404JSONResponse{NotFoundJSONResponse{Message: "no such target"}}, nil
	case errors.As(err, &conflict):
		return RetryTarget409JSONResponse{Message: conflict.Message}, nil
	case err != nil:
		return nil, err
	}
	return RetryTarget200JSONResponse(toPost(view)), nil
}

func (s *Server) GetPost(ctx context.Context, req GetPostRequestObject) (GetPostResponseObject, error) {
	view, err := s.svc.GetPost(ctx, userID(ctx), req.PostId)
	if errors.Is(err, service.ErrNotFound) {
		return GetPost404JSONResponse{NotFoundJSONResponse{Message: "no such post"}}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetPost200JSONResponse(toPost(view)), nil
}

func (s *Server) ReschedulePost(ctx context.Context, req ReschedulePostRequestObject) (ReschedulePostResponseObject, error) {
	view, err := s.svc.ReschedulePost(ctx, userID(ctx), req.PostId, req.Body.ScheduledAt)
	var conflict *service.Conflict
	switch {
	case errors.Is(err, service.ErrNotFound):
		return ReschedulePost404JSONResponse{NotFoundJSONResponse{Message: "no such post"}}, nil
	case errors.As(err, &conflict):
		return ReschedulePost409JSONResponse{Message: conflict.Message}, nil
	case err != nil:
		return nil, err
	}
	return ReschedulePost200JSONResponse(toPost(view)), nil
}

func (s *Server) CancelPost(ctx context.Context, req CancelPostRequestObject) (CancelPostResponseObject, error) {
	err := s.svc.CancelPost(ctx, userID(ctx), req.PostId)
	var conflict *service.Conflict
	switch {
	case errors.Is(err, service.ErrNotFound):
		return CancelPost404JSONResponse{NotFoundJSONResponse{Message: "no such post"}}, nil
	case errors.As(err, &conflict):
		return CancelPost409JSONResponse{Message: conflict.Message}, nil
	case err != nil:
		return nil, err
	}
	return CancelPost204Response{}, nil
}

func badRequest(err error) (string, bool) {
	var br *service.BadRequest
	if errors.As(err, &br) {
		return br.Message, true
	}
	return "", false
}

func toAccount(a service.Account) Account {
	out := Account{Id: a.ID, Platform: Platform(a.Platform), DisplayName: a.DisplayName, Brand: a.Brand, CreatedAt: a.CreatedAt}
	if a.NeedsReconnectAt.Valid {
		out.NeedsReconnect = new(true)
	}
	return out
}

func toPost(v service.PostView) Post {
	p := Post{
		Id:           v.Post.ID,
		Text:         v.Post.Text,
		FirstComment: v.Post.FirstComment,
		MediaIds:     v.MediaIDs,
		ScheduledAt:  v.Post.ScheduledAt,
		CreatedAt:    v.Post.CreatedAt,
		Targets:      []Target{},
	}
	for _, t := range v.Targets {
		p.Targets = append(p.Targets, toTarget(t))
	}
	return p
}

func toTarget(t db.PostTarget) Target {
	out := Target{
		Id:        t.ID,
		Platform:  Platform(t.Platform),
		Status:    TargetStatus(t.Status),
		LastError: t.LastError,
		RemoteUrl: t.RemoteUrl,
	}
	if t.Interrupted {
		out.Interrupted = &t.Interrupted
	}
	if t.AccountID.Valid {
		out.AccountId = &t.AccountID.UUID
	}
	if t.PublishedAt.Valid {
		out.PublishedAt = &t.PublishedAt.Time
	}
	if t.CommentStatus != nil {
		status := TargetCommentStatus(*t.CommentStatus)
		out.CommentStatus = &status
		out.CommentError = t.CommentError
		out.CommentUrl = t.CommentUrl
	}
	var options TargetOptions
	if json.Unmarshal(t.Options, &options) == nil && len(options) > 0 {
		out.Options = &options
	}
	return out
}

func postOptions(in []publisher.Option) []PostOption {
	out := make([]PostOption, 0, len(in))
	for _, o := range in {
		out = append(out, PostOption{Key: o.Key, Label: o.Label, MaxLength: o.MaxLength, Required: o.Required})
	}
	return out
}

func textCounting(m publisher.TextCounting) RulesTextCounting {
	switch m {
	case publisher.CountEmojiBytes:
		return EmojiBytes
	case publisher.CountXWeighted:
		return XWeighted
	}
	return CodePoints
}

func orZero(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}
