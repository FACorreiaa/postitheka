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
	svc *service.Service
	log *slog.Logger
}

var _ StrictServerInterface = (*Server)(nil)

// Mount registers the API on r, every route behind bearer authentication.
func Mount(r chi.Router, svc *service.Service, log *slog.Logger) http.Handler {
	s := &Server{svc: svc, log: log}
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
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		userID, err := s.svc.Authenticate(r.Context(), strings.TrimSpace(token))
		if errors.Is(err, service.ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unknown token")
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
				MediaTypes:         r.MediaTypes,
				RequiresText:       r.RequiresText,
				RequiresMedia:      r.RequiresMedia,
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
	up, err := s.svc.ReserveUpload(ctx, userID(ctx), req.Body.ContentType, req.Body.SizeBytes)
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
	in := service.CreatePostInput{
		AccountIDs:     req.Body.AccountIds,
		ScheduledAt:    req.Body.ScheduledAt,
		IdempotencyKey: req.Params.IdempotencyKey,
	}
	if req.Body.Text != nil {
		in.Text = *req.Body.Text
	}
	if req.Body.MediaIds != nil {
		in.MediaIDs = *req.Body.MediaIds
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

func badRequest(err error) (string, bool) {
	var br *service.BadRequest
	if errors.As(err, &br) {
		return br.Message, true
	}
	return "", false
}

func toAccount(a service.Account) Account {
	return Account{Id: a.ID, Platform: Platform(a.Platform), DisplayName: a.DisplayName, CreatedAt: a.CreatedAt}
}

func toPost(v service.PostView) Post {
	p := Post{
		Id:          v.Post.ID,
		Text:        v.Post.Text,
		MediaIds:    v.MediaIDs,
		ScheduledAt: v.Post.ScheduledAt,
		CreatedAt:   v.Post.CreatedAt,
		Targets:     []Target{},
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
	if t.AccountID.Valid {
		out.AccountId = &t.AccountID.UUID
	}
	if t.PublishedAt.Valid {
		out.PublishedAt = &t.PublishedAt.Time
	}
	return out
}

func textCounting(m publisher.TextCounting) RulesTextCounting {
	if m == publisher.CountEmojiBytes {
		return EmojiBytes
	}
	return CodePoints
}
