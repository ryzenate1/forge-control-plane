package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type discordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	Avatar        string `json:"avatar"`
	Email         string `json:"email"`
	Verified      bool   `json:"verified"`
}

type steamResponse struct {
	Response struct {
		Players []struct {
			SteamID     string `json:"steamid"`
			PersonaName string `json:"personaname"`
			AvatarFull  string `json:"avatarfull"`
			ProfileURL  string `json:"profileurl"`
		} `json:"players"`
	} `json:"response"`
}

type authentikUser struct {
	Sub       string `json:"sub"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Preferred string `json:"preferred_username"`
	Nickname  string `json:"nickname"`
}

type socialAuthState struct {
	State    string `json:"state"`
	Provider string `json:"provider"`
	Action   string `json:"action"`
	UserID   string `json:"userId,omitempty"`
	Expires  int64  `json:"expires"`
}

func registerSocialAuthRoutes(v1 fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	v1.Get("/auth/social/:provider", func(c *fiber.Ctx) error {
		return handleSocialAuthRedirect(c, cfg)
	})
	v1.Get("/auth/social/:provider/callback", func(c *fiber.Ctx) error {
		return handleSocialAuthCallback(c, cfg)
	})

	protected := v1.Group("", authMiddleware(cfg.AuthSecret, cfg.Store))
	protected.Get("/account/social/identities", func(c *fiber.Ctx) error {
		return listSocialIdentities(c, cfg)
	})
	protected.Post("/account/social/:provider/link", mutationLimiter, func(c *fiber.Ctx) error {
		return handleSocialLinkRedirect(c, cfg)
	})
	protected.Delete("/account/social/:provider/unlink", mutationLimiter, func(c *fiber.Ctx) error {
		return unlinkSocialIdentity(c, cfg)
	})

	admin := protected.Group("/admin", requireRole("admin"))
	admin.Get("/social/providers", func(c *fiber.Ctx) error {
		return listSocialProviders(c, cfg)
	})
	admin.Put("/social/providers/:id", mutationLimiter, func(c *fiber.Ctx) error {
		return updateSocialProvider(c, cfg)
	})
}

func handleSocialAuthRedirect(c *fiber.Ctx, cfg Config) error {
	provider := strings.ToLower(c.Params("provider"))
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "database required")
	}

	providers, err := cfg.Store.GetEnabledSocialProviders(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to get providers")
	}

	var sp *store.SocialProvider
	for _, p := range providers {
		if strings.EqualFold(p.Name, provider) {
			sp = &p
			break
		}
	}
	if sp == nil {
		return fiber.NewError(fiber.StatusNotFound, "social provider not found or disabled")
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate state")
	}
	state := hex.EncodeToString(stateBytes)

	stateKey := fmt.Sprintf("social:state:%s", state)
	stateData, _ := json.Marshal(socialAuthState{
		State:    state,
		Provider: provider,
		Action:   "login",
		Expires:  time.Now().Add(10 * time.Minute).Unix(),
	})

	if cfg.Redis != nil && cfg.RedisEnabled {
		cfg.Redis.Set(c.Context(), stateKey, string(stateData), 10*time.Minute)
	}

	redirectURI := fmt.Sprintf("%s/api/v1/auth/social/%s/callback", strings.TrimRight(cfg.PanelURL, "/"), provider)

	var authURL string
	switch provider {
	case "discord":
		authURL = fmt.Sprintf("https://discord.com/api/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			url.QueryEscape(sp.ClientID), url.QueryEscape(redirectURI), url.QueryEscape("identify email"), state)
	case "steam":
		realm := strings.TrimRight(cfg.PanelURL, "/")
		authURL = fmt.Sprintf("https://steamcommunity.com/openid/login?openid.ns=http://specs.openid.net/auth/2.0&openid.mode=checkid_setup&openid.return_to=%s&openid.realm=%s&openid.identity=http://specs.openid.net/auth/2.0/identifier_select&openid.claimed_id=http://specs.openid.net/auth/2.0/identifier_select",
			url.QueryEscape(redirectURI), url.QueryEscape(realm))
	case "authentik":
		authURL = fmt.Sprintf("%s/application/o/authorize/?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			strings.TrimRight(sp.IssuerURL, "/"), url.QueryEscape(sp.ClientID), url.QueryEscape(redirectURI), url.QueryEscape("openid email profile"), state)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "unsupported provider")
	}

	return c.Redirect(authURL, fiber.StatusFound)
}

func handleSocialAuthCallback(c *fiber.Ctx, cfg Config) error {
	provider := strings.ToLower(c.Params("provider"))
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "database required")
	}

	code := c.Query("code")
	state := c.Query("state")

	stateKey := fmt.Sprintf("social:state:%s", state)
	var storedState socialAuthState

	if cfg.Redis != nil && cfg.RedisEnabled {
		data, err := cfg.Redis.Get(c.Context(), stateKey).Result()
		if err != nil || data == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid state")
		}
		json.Unmarshal([]byte(data), &storedState)
		cfg.Redis.Del(c.Context(), stateKey)
	} else {
		return fiber.NewError(fiber.StatusServiceUnavailable, "state validation unavailable")
	}

	if storedState.Provider != provider || storedState.Expires < time.Now().Unix() {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired state")
	}

	var email, providerID, providerName, avatarURL, profileURL string

	switch provider {
	case "discord":
		user, err := exchangeDiscordCode(c.Context(), cfg, code)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}
		providerID = user.ID
		providerName = user.Username
		email = user.Email
		if user.Avatar != "" {
			avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", user.ID, user.Avatar)
		}
		profileURL = fmt.Sprintf("https://discordapp.com/users/%s", user.ID)
	case "steam":
		claimedID := c.Query("openid.claimed_id")
		if !strings.HasPrefix(claimedID, "https://steamcommunity.com/openid/id/") {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid Steam identity")
		}
		steamID := strings.TrimPrefix(claimedID, "https://steamcommunity.com/openid/id/")

		if err := verifySteamAssertion(c, cfg); err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}

		player, err := fetchSteamPlayerSummary(c.Context(), cfg, steamID)
		if err == nil && len(player.Response.Players) > 0 {
			p := player.Response.Players[0]
			providerID = p.SteamID
			providerName = p.PersonaName
			avatarURL = p.AvatarFull
			profileURL = p.ProfileURL
		}
	case "authentik":
		user, err := exchangeAuthentikCode(c.Context(), cfg, code)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}
		providerID = user.Sub
		providerName = user.Preferred
		if providerName == "" {
			providerName = user.Nickname
		}
		if providerName == "" {
			providerName = user.Name
		}
		email = user.Email
	default:
		return fiber.NewError(fiber.StatusBadRequest, "unsupported provider")
	}

	if storedState.Action == "link" {
		if storedState.UserID == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid link state")
		}
		existingIdentity, err := cfg.Store.GetSocialIdentity(c.Context(), provider, providerID)
		if err == nil && existingIdentity.UserID != storedState.UserID {
			return fiber.NewError(fiber.StatusConflict, "social identity is already linked to another account")
		}

		avURL := &avatarURL
		if avatarURL == "" {
			avURL = nil
		}
		prURL := &profileURL
		if profileURL == "" {
			prURL = nil
		}
		if _, err := cfg.Store.LinkSocialIdentity(c.Context(), storedState.UserID, provider, providerID, providerName, avURL, prURL); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to link social identity")
		}
		return c.Redirect(fmt.Sprintf("%s/account", strings.TrimRight(cfg.PanelURL, "/")), fiber.StatusFound)
	}

	existingIdentity, err := cfg.Store.GetSocialIdentity(c.Context(), provider, providerID)
	if err == nil && existingIdentity != nil {
		user, err := cfg.Store.GetUserByID(c.Context(), existingIdentity.UserID)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "linked user not found")
		}
		if user.Disabled {
			return fiber.NewError(fiber.StatusForbidden, "account is disabled")
		}

		token, err := issueToken(cfg.AuthSecret, user)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue token")
		}
		csrfToken, _ := generateCSRFToken()
		expires := time.Now().Add(tokenTTL)
		setSessionCookies(c, token, csrfToken, expires)

		code, _ = issueExchangeCode(token, csrfToken)
		redirectURL := strings.TrimRight(cfg.PanelURL, "/")
		if code != "" {
			redirectURL += "/?code=" + code
		}
		return c.Redirect(redirectURL, fiber.StatusFound)
	}

	if email == "" {
		return c.Redirect(fmt.Sprintf("%s/auth/social/link?provider=%s&providerId=%s&providerName=%s",
			strings.TrimRight(cfg.PanelURL, "/"), provider, providerID, url.QueryEscape(providerName)), fiber.StatusFound)
	}

	user, err := cfg.Store.GetUserByEmail(c.Context(), email)
	if err != nil {
		randomPass := make([]byte, 24)
		rand.Read(randomPass)
		passStr := hex.EncodeToString(randomPass)
		hashedPass, _ := bcrypt.GenerateFromPassword([]byte(passStr), store.BcryptCost())

		newUser, err := cfg.Store.CreateUser(c.Context(), store.CreateUserRequest{
			Email:    email,
			Password: string(hashedPass),
			Role:     "user",
		}, nil)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to create user")
		}

		avURL := &avatarURL
		if avatarURL == "" {
			avURL = nil
		}
		prURL := &profileURL
		if profileURL == "" {
			prURL = nil
		}

		cfg.Store.LinkSocialIdentity(c.Context(), newUser.ID, provider, providerID, providerName, avURL, prURL)

		token, err := issueToken(cfg.AuthSecret, newUser)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue token")
		}
		csrfToken, _ := generateCSRFToken()
		expires := time.Now().Add(tokenTTL)
		setSessionCookies(c, token, csrfToken, expires)

		code, _ = issueExchangeCode(token, csrfToken)
		redirectURL := strings.TrimRight(cfg.PanelURL, "/")
		if code != "" {
			redirectURL += "/?code=" + code
		}
		return c.Redirect(redirectURL, fiber.StatusFound)
	}

	avURL := &avatarURL
	if avatarURL == "" {
		avURL = nil
	}
	prURL := &profileURL
	if profileURL == "" {
		prURL = nil
	}

	cfg.Store.LinkSocialIdentity(c.Context(), user.ID, provider, providerID, providerName, avURL, prURL)

	token, err := issueToken(cfg.AuthSecret, user)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not issue token")
	}
	csrfToken, _ := generateCSRFToken()
	expires := time.Now().Add(tokenTTL)
	setSessionCookies(c, token, csrfToken, expires)

	code, _ = issueExchangeCode(token, csrfToken)
	redirectURL := strings.TrimRight(cfg.PanelURL, "/")
	if code != "" {
		redirectURL += "/?code=" + code
	}
	return c.Redirect(redirectURL, fiber.StatusFound)
}

func exchangeDiscordCode(ctx context.Context, cfg Config, code string) (*discordUser, error) {
	redirectURI := fmt.Sprintf("%s/api/v1/auth/social/discord/callback", strings.TrimRight(cfg.PanelURL, "/"))

	providers, err := cfg.Store.GetSocialProviders(ctx)
	if err != nil {
		return nil, err
	}

	var clientID, clientSecret string
	for _, p := range providers {
		if p.Name == "discord" {
			clientID = p.ClientID
			clientSecret = p.ClientSecret
			break
		}
	}
	if clientID == "" {
		return nil, fmt.Errorf("discord provider not configured")
	}

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("scope", "identify email")

	tokenResp, err := http.PostForm("https://discord.com/api/oauth2/token", data)
	if err != nil {
		return nil, err
	}
	defer tokenResp.Body.Close()

	var tokenResult struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://discord.com/api/users/@me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResult.AccessToken)

	userResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer userResp.Body.Close()

	var user discordUser
	if err := json.NewDecoder(userResp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func verifySteamAssertion(c *fiber.Ctx, cfg Config) error {
	openIDMode := c.Query("openid.mode")
	openIDNS := c.Query("openid.ns")
	if openIDMode != "id_res" || openIDNS != "http://specs.openid.net/auth/2.0" {
		return fmt.Errorf("invalid OpenID response")
	}

	params := url.Values{}
	c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
		params.Set(string(key), string(value))
	})
	params.Set("openid.mode", "check_authentication")

	resp, err := http.PostForm("https://steamcommunity.com/openid/login", params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "is_valid:true") {
		return fmt.Errorf("Steam authentication verification failed")
	}

	return nil
}

func fetchSteamPlayerSummary(ctx context.Context, cfg Config, steamID string) (*steamResponse, error) {
	providers, err := cfg.Store.GetSocialProviders(ctx)
	if err != nil {
		return nil, err
	}

	var apiKey string
	for _, p := range providers {
		if p.Name == "steam" {
			apiKey = p.ClientSecret
			break
		}
	}
	if apiKey == "" {
		return nil, fmt.Errorf("steam API key not configured")
	}

	reqURL := fmt.Sprintf("https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v0002/?key=%s&steamids=%s", apiKey, steamID)
	resp, err := http.DefaultClient.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result steamResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func exchangeAuthentikCode(ctx context.Context, cfg Config, code string) (*authentikUser, error) {
	providers, err := cfg.Store.GetSocialProviders(ctx)
	if err != nil {
		return nil, err
	}

	var baseURL, oauthClientID, oauthClientSecret string
	for _, p := range providers {
		if p.Name == "authentik" {
			baseURL = p.IssuerURL
			oauthClientID = p.ClientID
			oauthClientSecret = p.ClientSecret
			break
		}
	}
	if baseURL == "" || oauthClientID == "" || oauthClientSecret == "" {
		return nil, fmt.Errorf("authentik provider not configured")
	}

	redirectURI := fmt.Sprintf("%s/api/v1/auth/social/authentik/callback", strings.TrimRight(cfg.PanelURL, "/"))

	data := url.Values{}
	data.Set("client_id", oauthClientID)
	data.Set("client_secret", oauthClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	tokenURL := fmt.Sprintf("%s/application/o/token/", strings.TrimRight(baseURL, "/"))
	tokenResp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return nil, err
	}
	defer tokenResp.Body.Close()

	var tokenResult struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		return nil, err
	}

	userURL := fmt.Sprintf("%s/application/o/userinfo/", strings.TrimRight(baseURL, "/"))
	req, _ := http.NewRequestWithContext(ctx, "GET", userURL, nil)
	req.Header.Set("Authorization", "Bearer "+tokenResult.AccessToken)

	userResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer userResp.Body.Close()

	var user authentikUser
	if err := json.NewDecoder(userResp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func listSocialIdentities(c *fiber.Ctx, cfg Config) error {
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing user")
	}

	ctx, cancel := requestContext()
	defer cancel()

	identities, err := cfg.Store.ListSocialIdentities(ctx, claims.Sub)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(identities)
}

func handleSocialLinkRedirect(c *fiber.Ctx, cfg Config) error {
	provider := strings.ToLower(c.Params("provider"))
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "database required")
	}

	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing user")
	}

	providers, err := cfg.Store.GetEnabledSocialProviders(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to get providers")
	}

	var sp *store.SocialProvider
	for _, p := range providers {
		if strings.EqualFold(p.Name, provider) {
			sp = &p
			break
		}
	}
	if sp == nil {
		return fiber.NewError(fiber.StatusNotFound, "social provider not found or disabled")
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate state")
	}
	state := hex.EncodeToString(stateBytes)

	stateKey := fmt.Sprintf("social:state:%s", state)
	stateData, _ := json.Marshal(socialAuthState{
		State:    state,
		Provider: provider,
		Action:   "link",
		UserID:   claims.Sub,
		Expires:  time.Now().Add(10 * time.Minute).Unix(),
	})

	if cfg.Redis != nil && cfg.RedisEnabled {
		cfg.Redis.Set(c.Context(), stateKey, string(stateData), 10*time.Minute)
	}

	redirectURI := fmt.Sprintf("%s/api/v1/auth/social/%s/callback", strings.TrimRight(cfg.PanelURL, "/"), provider)

	var authURL string
	switch provider {
	case "discord":
		authURL = fmt.Sprintf("https://discord.com/api/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			url.QueryEscape(sp.ClientID), url.QueryEscape(redirectURI), url.QueryEscape("identify email"), state)
	case "steam":
		realm := strings.TrimRight(cfg.PanelURL, "/")
		authURL = fmt.Sprintf("https://steamcommunity.com/openid/login?openid.ns=http://specs.openid.net/auth/2.0&openid.mode=checkid_setup&openid.return_to=%s&openid.realm=%s&openid.identity=http://specs.openid.net/auth/2.0/identifier_select&openid.claimed_id=http://specs.openid.net/auth/2.0/identifier_select",
			url.QueryEscape(redirectURI), url.QueryEscape(realm))
	case "authentik":
		authURL = fmt.Sprintf("%s/application/o/authorize/?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			strings.TrimRight(sp.IssuerURL, "/"), url.QueryEscape(sp.ClientID), url.QueryEscape(redirectURI), url.QueryEscape("openid email profile"), state)
	}

	return c.Redirect(authURL, fiber.StatusFound)
}

func unlinkSocialIdentity(c *fiber.Ctx, cfg Config) error {
	provider := strings.ToLower(c.Params("provider"))
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing user")
	}

	ctx, cancel := requestContext()
	defer cancel()

	if err := cfg.Store.UnlinkSocialIdentity(ctx, claims.Sub, provider); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{"ok": true})
}

type socialProviderResponse struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	DisplayName     string    `json:"displayName"`
	Enabled         bool      `json:"enabled"`
	ClientID        string    `json:"clientId"`
	IssuerURL       string    `json:"issuerUrl,omitempty"`
	HasClientSecret bool      `json:"hasClientSecret"`
	Scopes          []string  `json:"scopes"`
	ButtonStyle     string    `json:"buttonStyle"`
	IconClass       string    `json:"iconClass"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func socialProviderPublicView(provider store.SocialProvider) socialProviderResponse {
	return socialProviderResponse{
		ID: provider.ID, Name: provider.Name, DisplayName: provider.DisplayName, Enabled: provider.Enabled,
		ClientID: provider.ClientID, IssuerURL: provider.IssuerURL, HasClientSecret: provider.ClientSecret != "",
		Scopes: provider.Scopes, ButtonStyle: provider.ButtonStyle, IconClass: provider.IconClass,
		CreatedAt: provider.CreatedAt, UpdatedAt: provider.UpdatedAt,
	}
}

func listSocialProviders(c *fiber.Ctx, cfg Config) error {
	ctx, cancel := requestContext()
	defer cancel()

	providers, err := cfg.Store.GetSocialProviders(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	response := make([]socialProviderResponse, 0, len(providers))
	for _, provider := range providers {
		response = append(response, socialProviderPublicView(provider))
	}
	return c.JSON(response)
}

func updateSocialProvider(c *fiber.Ctx, cfg Config) error {
	var req struct {
		Enabled      *bool    `json:"enabled"`
		ClientID     *string  `json:"clientId"`
		ClientSecret *string  `json:"clientSecret"`
		IssuerURL    *string  `json:"issuerUrl"`
		Scopes       []string `json:"scopes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	ctx, cancel := requestContext()
	defer cancel()

	provider, err := cfg.Store.UpdateSocialProvider(ctx, c.Params("id"), req.Enabled, req.ClientID, req.ClientSecret, req.IssuerURL, req.Scopes)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(socialProviderPublicView(*provider))
}
