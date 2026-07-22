package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// defaultRateLimitPenalty s'applique quand l'exchange ne fournit pas
// d'en-tête Retry-After exploitable.
const defaultRateLimitPenalty = 5 * time.Second

// sign calcule la signature HMAC-SHA256 hexadécimale du payload.
func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// doPublic exécute une requête non signée (endpoints publics).
func (c *Client) doPublic(ctx context.Context, method, path string, params url.Values, out any) error {
	endpoint := c.cfg.RESTBaseURL + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return fmt.Errorf("requête %s: %w", path, err)
	}
	return c.execute(req, path, out)
}

// doSigned exécute une requête signée : timestamp + recvWindow ajoutés,
// signature HMAC-SHA256 du query string, en-tête X-MBX-APIKEY.
// La signature et la clé ne sont jamais journalisées.
func (c *Client) doSigned(ctx context.Context, method, path string, params url.Values, out any) error {
	if c.cfg.APIKey == "" || c.cfg.APISecret == "" {
		return fmt.Errorf("clés API absentes (BINANCE_API_KEY / BINANCE_API_SECRET)")
	}
	if err := c.limiter.wait(ctx); err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}

	if params == nil {
		params = url.Values{}
	}
	params.Set("timestamp", strconv.FormatInt(c.now().UnixMilli(), 10))
	params.Set("recvWindow", strconv.FormatInt(c.cfg.RecvWindow.Milliseconds(), 10))

	payload := params.Encode()
	signature := sign(payload, c.cfg.APISecret)
	endpoint := c.cfg.RESTBaseURL + path + "?" + payload + "&signature=" + signature

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return fmt.Errorf("requête %s: %w", path, err)
	}
	req.Header.Set("X-MBX-APIKEY", c.cfg.APIKey)
	return c.execute(req, path, out)
}

// execute envoie la requête, applique la gestion 429/418 (pénalité
// adaptative du token bucket) et décode la réponse JSON dans out.
func (c *Client) execute(req *http.Request, path string, out any) error {
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("appel %s: %w", path, err)
	}
	defer resp.Body.Close()

	c.log.Debug("appel REST", "method", req.Method, "path", path,
		"status", resp.StatusCode, "latency", time.Since(start).String())

	if resp.StatusCode == http.StatusOK {
		if out == nil {
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("décodage %s: %w", path, err)
		}
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	apiErr := &APIError{Status: resp.StatusCode}
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(body, &payload) == nil {
		apiErr.Code = payload.Code
		apiErr.Msg = payload.Msg
	} else {
		apiErr.Msg = string(body)
	}

	// 429 : trop de requêtes ; 418 : bannissement IP temporaire.
	// Le token bucket est gelé pour la durée demandée par l'exchange.
	if resp.StatusCode == 429 || resp.StatusCode == 418 {
		penalty := defaultRateLimitPenalty
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				penalty = time.Duration(secs) * time.Second
			}
		}
		c.limiter.penalize(penalty)
		c.log.Warn("rate limit atteint, délai adaptatif appliqué",
			"status", resp.StatusCode, "penalty", penalty.String())
	}

	return apiErr
}
