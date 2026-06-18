package a2a

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// principalContextKey 用于在 context 中存放已认证主体。
type principalContextKey struct{}

// WithPrincipal 将 Principal 注入 context。
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// PrincipalFromContext 从 context 中提取 Principal。
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(*Principal)
	return p, ok
}

// GRPCAuthFunc 从 gRPC context 中提取并验证凭证，返回 Principal。
type GRPCAuthFunc func(ctx context.Context) (*Principal, error)

// UnaryAuthInterceptor 返回一个 unary interceptor。
func UnaryAuthInterceptor(auth GRPCAuthFunc) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if auth == nil {
			return handler(ctx, req)
		}
		p, err := auth(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		ctx = WithPrincipal(ctx, p)
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor 返回一个 stream interceptor。
func StreamAuthInterceptor(auth GRPCAuthFunc) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if auth == nil {
			return handler(srv, ss)
		}
		p, err := auth(ss.Context())
		if err != nil {
			return status.Error(codes.Unauthenticated, err.Error())
		}
		ctx := WithPrincipal(ss.Context(), p)
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// APIKeyAuthFunc 从 metadata 的 header 中提取 API Key 并校验。
func APIKeyAuthFunc(keys map[string]string, headerName string) GRPCAuthFunc {
	if headerName == "" {
		headerName = "x-api-key"
	}
	return func(ctx context.Context) (*Principal, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, ErrAuthHeaderMissing
		}
		values := md.Get(headerName)
		if len(values) == 0 {
			return nil, ErrAuthHeaderMissing
		}
		principalID, ok := keys[values[0]]
		if !ok {
			return nil, errors.New("无效 API Key")
		}
		return &Principal{ID: principalID, Scopes: []string{"*"}}, nil
	}
}

// BearerAuthFunc 从 metadata 的 authorization 头中提取 Bearer token。
func BearerAuthFunc(validate BearerTokenValidator) GRPCAuthFunc {
	return func(ctx context.Context) (*Principal, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, ErrAuthHeaderMissing
		}
		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, ErrAuthHeaderMissing
		}
		header := values[0]
		if !strings.HasPrefix(header, "Bearer ") {
			return nil, ErrAuthBearerRequired
		}
		return validate(strings.TrimPrefix(header, "Bearer "))
	}
}
