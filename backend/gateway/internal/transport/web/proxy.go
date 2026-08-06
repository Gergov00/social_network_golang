package web

import (
	"fmt"
	"net/http/httputil"
	"net/url"
)

// NewAuthProxy проксирует /auth/* в auth-service как есть.
//
// Почему прокси, а не свои хендлеры: auth уже отдаёт REST/JSON и сам ставит
// httpOnly-cookie с нужными Path/SameSite/Secure. Пересобирать ответ в
// gateway значило бы держать логику cookie в двух местах и ломать сессии при
// каждом расхождении.
func NewAuthProxy(target string) (*httputil.ReverseProxy, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		// net/url does not export a "missing scheme/host" sentinel error, so we
		// wrap url.Error ourselves with a message that says what's actually wrong.
		return nil, &url.Error{Op: "parse", URL: target, Err: fmt.Errorf("missing protocol scheme or host")}
	}

	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(parsed)
			// Путь сохраняем как есть: auth слушает те же /auth/*.
			r.Out.URL.Path = r.In.URL.Path
			// Host НЕ пробрасываем с клиента: SetURL уже поставил Host
			// таргета. Client-controlled Host — классический вектор
			// Host-header-injection; тем, кому нужен исходный хост, его
			// отдаёт SetXForwarded() через X-Forwarded-Host.
			r.SetXForwarded()
		},
	}, nil
}
