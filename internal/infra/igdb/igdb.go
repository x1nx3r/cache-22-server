package igdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	twitchURL = "https://id.twitch.tv/oauth2/token"
	apiURL    = "https://api.igdb.com/v4"
)

type Client struct {
	clientID string
	secret   string
	apiBase  string
	authBase string
	http     *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func New(clientID, secret string) *Client {
	return NewWithBases(clientID, secret, apiURL, twitchURL)
}

func NewWithBases(clientID, secret, apiBase, authBase string) *Client {
	return &Client{clientID: clientID, secret: secret, apiBase: apiBase, authBase: authBase, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) Enabled() bool {
	return c != nil && c.clientID != "" && c.secret != ""
}

func (c *Client) tokenLocked() (string, error) {
	if c.token != "" && time.Now().Add(60*time.Second).Before(c.expiry) {
		return c.token, nil
	}
	req, err := http.NewRequest("POST", c.authBase+fmt.Sprintf("?client_id=%s&client_secret=%s&grant_type=client_credentials", c.clientID, c.secret), nil)
	if err != nil {
		return "", err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitch token: %s", res.Status)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	c.token = out.AccessToken
	c.expiry = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return c.token, nil
}

func (c *Client) query(path, body string, v any) error {
	c.mu.Lock()
	token, err := c.tokenLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.apiBase+path, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("igdb %s: %s %s", path, res.Status, strings.TrimSpace(string(raw)))
	}
	return json.NewDecoder(res.Body).Decode(v)
}

func (c *Client) PlatformID(name string) (int64, error) {
	var out []struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		Abbreviation string `json:"abbreviation"`
	}
	if err := c.query("/platforms", fmt.Sprintf(`search "%s"; fields id,name,abbreviation; limit 10;`, name), &out); err != nil {
		return 0, err
	}
	for _, p := range out {
		if strings.EqualFold(p.Name, name) || strings.EqualFold(p.Abbreviation, name) {
			return p.ID, nil
		}
	}
	if len(out) > 0 {
		return out[0].ID, nil
	}
	return 0, fmt.Errorf("platform %q not found", name)
}

type Game struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	CoverURL string `json:"cover_url"`
}

func (c *Client) Search(title string, platformID int64) ([]Game, error) {
	var out []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Cover *struct {
			URL string `json:"url"`
		} `json:"cover"`
	}
	body := fmt.Sprintf(`search "%s"; fields name,cover.url; where platforms = (%d); limit 5;`, escapeQuery(title), platformID)
	if err := c.query("/games", body, &out); err != nil {
		return nil, err
	}
	var games []Game
	for _, g := range out {
		game := Game{ID: g.ID, Name: g.Name}
		if g.Cover != nil {
			game.CoverURL = CoverBig(g.Cover.URL)
		}
		games = append(games, game)
	}
	return games, nil
}

func escapeQuery(s string) string {
	return strings.ReplaceAll(s, `"`, "")
}

func CoverBig(url string) string {
	u := strings.Replace(url, "/t_thumb/", "/t_cover_big/", 1)
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	return u
}

func CleanTitle(serial string) string {
	s := strings.TrimSpace(serial)
	for {
		if !strings.HasSuffix(s, ")") {
			return s
		}
		i := strings.LastIndex(s, "(")
		if i <= 0 {
			return s
		}
		s = strings.TrimSpace(s[:i])
	}
}

func (c *Client) Download(url, dest string) error {
	res, err := c.http.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("cover download: %s", res.Status)
	}
	return writeLimited(res.Body, dest)
}

func writeLimited(r io.Reader, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(r, 8<<20)); err != nil {
		return err
	}
	if buf.Len() == 0 {
		return fmt.Errorf("empty cover")
	}
	return os.WriteFile(dest, buf.Bytes(), 0o644)
}
