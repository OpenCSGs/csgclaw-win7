package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"csgclaw/internal/apps"
	"csgclaw/internal/auth"
	"csgclaw/internal/connectorcli"
)

type win7OfflinePackageTransport struct{}

func (win7OfflinePackageTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("package upgrade unexpectedly attempted a download")
}

func TestWin7LiveCachedConnectorUpgrade(t *testing.T) {
	oldRoot, newRoot := os.Getenv("CSGCLAW_TEST_OLD_CONNECTOR_PACKAGES"), os.Getenv("CSGCLAW_TEST_CONNECTOR_PACKAGES")
	if oldRoot == "" || newRoot == "" {
		t.Skip("set both old and updated shipped connector package directories")
	}
	manifests, err := filepath.Glob(filepath.Join(newRoot, "lark-cli", "*", "package.json"))
	if err != nil || len(manifests) != 1 {
		t.Fatal("expected one shipped Feishu CLI version")
	}
	data, err := os.ReadFile(manifests[0])
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
		Skills  string `json:"skills_commit"`
		Binary  string `json:"executable_sha256"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	archive := connectorcli.Archive{URL: "https://unused.invalid/fixture", SHA256: strings.Repeat("0", 64)}
	installer := connectorcli.Installer{
		Root: t.TempDir(), PreinstalledDir: oldRoot,
		Client: &http.Client{Transport: win7OfflinePackageTransport{}},
		Spec: connectorcli.InstallSpec{Executable: "lark-cli", Version: manifest.Version, VersionArgs: []string{"-v"},
			Releases: map[string]connectorcli.Archive{runtime.GOOS + "-" + runtime.GOARCH: archive},
			Skills:   connectorcli.SkillArchive{Archive: archive, Version: manifest.Skills, Prefix: "lark-"}},
	}
	installed, err := installer.Install(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	oldData, err := os.ReadFile(installed.Executable)
	if err != nil {
		t.Fatal(err)
	}
	oldDigest := sha256.Sum256(oldData)
	if hex.EncodeToString(oldDigest[:]) == manifest.Binary {
		t.Fatal("old package fixture already contains the updated binary")
	}
	installer.PreinstalledDir = newRoot
	installed, err = installer.Install(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	newData, err := os.ReadFile(installed.Executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(newData)
	if hex.EncodeToString(digest[:]) != manifest.Binary {
		t.Fatal("valid cached CLI was not upgraded to the shipped Win7 rebuild")
	}
}

func TestWin7LiveMCPMarketplace(t *testing.T) {
	path := os.Getenv("CSGCLAW_TEST_MCP_AUTH_FILE")
	if path == "" {
		t.Skip("set CSGCLAW_TEST_MCP_AUTH_FILE to an isolated Hub credential fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var credentials struct {
		HubURL      string `json:"hub_url"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &credentials); err != nil {
		t.Fatal("invalid Hub credential fixture")
	}
	hub, err := url.Parse(credentials.HubURL)
	if err != nil || hub.Scheme != "https" || hub.Host == "" || hub.User != nil || credentials.AccessToken == "" {
		t.Fatal("Hub fixture requires its HTTPS origin and an access token")
	}
	// Official Hub credentials belong to the account store, rather than the
	// editable registry configuration. Keep the real request isolated from the
	// host's existing account and bind the credential to this fixture's site.
	t.Setenv("CSGHUB_API_BASE_URL", credentials.HubURL)
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))
	previousToken := remoteMCPHubAccessToken
	remoteMCPHubAccessToken = func() (string, error) { return credentials.AccessToken, nil }
	t.Cleanup(func() { remoteMCPHubAccessToken = previousToken })
	handler := &Handler{serverNoAuth: true}
	handler.SetConfigPath(filepath.Join(t.TempDir(), "config.toml"))
	server := httptest.NewServer(handler.Routes())
	t.Cleanup(server.Close)
	response, err := (&http.Client{Timeout: 30 * time.Second}).Get(server.URL + "/api/v1/mcp-servers/remote?page=1&per=12")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("MCP marketplace: HTTP %d", response.StatusCode)
	}
	var page remoteMCPServersListResponse
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil || page.Page != 1 {
		t.Fatalf("MCP marketplace did not return a valid page: decode=%v", err)
	}
	t.Logf("MCP marketplace: %d resources on page 1", len(page.Items))
}

func TestWin7LiveFeishuWebsocketTLS(t *testing.T) {
	if os.Getenv("CSGCLAW_TEST_CONNECTOR_PACKAGES") == "" {
		t.Skip("set CSGCLAW_TEST_CONNECTOR_PACKAGES to enable live Feishu checks")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	// An anonymous upgrade to the public Feishu origin should reach HTTP. This
	// checks the SDK's TLS dialer without connecting an application or account.
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, "wss://open.feishu.cn/", nil)
	if response == nil || (err != nil && !errors.Is(err, websocket.ErrBadHandshake)) {
		t.Fatalf("Feishu WebSocket TLS did not reach HTTP: %v", err)
	}
	response.Body.Close()
	if conn != nil {
		conn.Close()
	}
	t.Logf("verified Feishu WebSocket TLS, anonymous upgrade HTTP %d", response.StatusCode)
}

// Exercise the shipped CLI and the same HTTP operation used by the Connect
// button, without granting either application or personal account access.
func TestWin7LiveFeishuAuthorization(t *testing.T) {
	packages := os.Getenv("CSGCLAW_TEST_CONNECTOR_PACKAGES")
	if packages == "" {
		t.Skip("set CSGCLAW_TEST_CONNECTOR_PACKAGES to the shipped connector-packages directory")
	}
	t.Setenv("CSGCLAW_CONNECTOR_PACKAGES_DIR", packages)
	service, err := apps.NewService(filepath.Join(t.TempDir(), "state.json"), apps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	resource, err := service.EnsureFeishuCLI(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{apps: service, serverNoAuth: true}
	server := httptest.NewServer(h.Routes())
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 10 * time.Second}
	base := server.URL + "/api/v1/connectors/resources/" + resource.InstallationID

	for _, operationID := range []string{"initial-connect", "retry-after-cancel"} {
		resource, err = service.Get(t.Context(), "", resource.InstallationID)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(apps.CLIRequest{OperationID: operationID, Revision: resource.Revision})
		response, err := client.Post(base+"/cli/connect", "application/json", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		var op apps.CLIOperation
		err = json.NewDecoder(response.Body).Decode(&op)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusAccepted {
			t.Fatalf("start authorization: HTTP %d, decode=%v", response.StatusCode, err)
		}
		session := base + "/cli/sessions/" + op.SessionID
		deadline := time.Now().Add(2 * time.Minute)
		for op.URL == "" {
			if op.Status == "failed" || op.Status == "error" || time.Now().After(deadline) {
				t.Fatalf("authorization stopped: status=%s stage=%s error=%s", op.Status, op.Stage, op.ErrorCode)
			}
			time.Sleep(100 * time.Millisecond)
			response, err = client.Get(session)
			if err != nil {
				t.Fatal(err)
			}
			err = json.NewDecoder(response.Body).Decode(&op)
			response.Body.Close()
			if err != nil || response.StatusCode != http.StatusOK {
				t.Fatalf("poll authorization: HTTP %d, decode=%v", response.StatusCode, err)
			}
		}
		authorizationURL, err := url.Parse(op.URL)
		if err != nil || authorizationURL.Host != "open.feishu.cn" || authorizationURL.Path != "/page/launcher" || authorizationURL.Query().Get("user_code") == "" {
			t.Fatal("authorization did not produce the Feishu launcher URL")
		}
		request, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, session, nil)
		response, err = client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("cancel authorization: HTTP %d", response.StatusCode)
		}
		deadline = time.Now().Add(20 * time.Second)
		for {
			op, err = service.CLISession(t.Context(), resource.InstallationID, op.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if op.Status == "cancelled" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("authorization did not cancel: status=%s stage=%s", op.Status, op.Stage)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
