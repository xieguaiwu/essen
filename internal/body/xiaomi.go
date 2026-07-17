package body

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"essen/internal/models"
)

// ---------------------------------------------------------------------------
// Xiaomi cloud API client for Mi Scale data
//
// Reference: https://josenobile.co/guides/xiaomi-scale-api/
//            https://github.com/AlexxIT/SmartScaleConnect
//
// Flow:
//   1. Login to Xiaomi account → get serviceToken + ssecurity
//   2. Sign API request with ssecurity (HMAC-SHA1 + RC4)
//   3. POST to scale API endpoint → parse response
// ---------------------------------------------------------------------------

// xiaomiSession holds authentication state after login.
type xiaomiSession struct {
	UserID       string
	ServiceToken string
	SSecurity    string // shared secret for API signing
	DeviceID     string
	Client       *http.Client
}

// xiaomiScaleRecord maps the JSON response from Xiaomi scale API.
type xiaomiScaleRecord struct {
	Timestamp int64   `json:"ts"`
	Weight    float64 `json:"weight"`
	BMI       float64 `json:"bmi"`
	BodyFat   float64 `json:"bodyfat"`
	Muscle    float64 `json:"muscle"`
	Bone      float64 `json:"bone"`
	Water     float64 `json:"water"`
	BMR       float64 `json:"bmr"`
	VisFat    int     `json:"visfat"`
	Score     int     `json:"score"`
	BodyAge   int     `json:"bodyage"`
}

// xiaomiAPIResponse is the top-level wrapper from Xiaomi Home API.
type xiaomiAPIResponse struct {
	Result struct {
		Code int                `json:"code"`
		Data []xiaomiScaleRecord `json:"data"`
	} `json:"result"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// FetchXiaomi performs a full Xiaomi cloud sync:
//   1. Login using Mi Account credentials
//   2. Fetch all scale measurements from the past 90 days
//   3. Parse and return as BodyMeasurement slice
//
// userID can be email or phone number; password is the Mi Account password.
func FetchXiaomi(userID, password string) ([]models.BodyMeasurement, error) {
	sess, err := loginXiaomi(userID, password)
	if err != nil {
		return nil, fmt.Errorf("小米登录失败: %w", err)
	}

	records, err := fetchScaleData(sess, 90)
	if err != nil {
		return nil, fmt.Errorf("获取体测数据失败: %w", err)
	}

	return recordsToMeasurements(records)
}

// ---------------------------------------------------------------------------
// Login flow
// ---------------------------------------------------------------------------

func loginXiaomi(userID, password string) (*xiaomiSession, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}

	client.CheckRedirect = nil // default redirect policy

	// Step 1: GET serviceLogin to get _sign
	loginURL := "https://account.xiaomi.com/pass/serviceLogin?sid=xiaomi&_json=true"
	req, err := http.NewRequest(http.MethodGet, loginURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建登录请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求登录页面失败: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	resp.Body.Close()

	// Extract _sign from JSON response (Xiaomi returns a JSONP-like response)
	signRe := regexp.MustCompile(`"_sign"\s*:\s*"([^"]+)"`)
	signMatch := signRe.FindStringSubmatch(string(body))
	if len(signMatch) < 2 {
		return nil, fmt.Errorf("无法获取 _sign 参数，请检查网络连接")
	}
	sign := signMatch[1]

	// Step 2: POST to serviceLoginAuth2 with credentials
	data := url.Values{}
	data.Set("_json", "true")
	data.Set("sid", "xiaomi")
	data.Set("user", userID)
	data.Set("hash", sha1Upper(password))
	data.Set("_sign", sign)

	authURL := "https://account.xiaomi.com/pass/serviceLoginAuth2"
	req, err = http.NewRequest(http.MethodPost, authURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建认证请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("认证请求失败: %w", err)
	}
	authBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	resp.Body.Close()

	// The response is JSON with redirect URL
	var authResp struct {
		Code      int    `json:"code"`
		Desc      string `json:"desc"`
		Location  string `json:"location"`
		UserID    string `json:"userId"`
		PassToken string `json:"passToken"`
		SSecurity string `json:"ssecurity"`
	}
	if err := json.Unmarshal(authBody, &authResp); err != nil {
		return nil, fmt.Errorf("解析认证响应失败: %w", err)
	}
	if authResp.Code != 0 {
		return nil, fmt.Errorf("认证失败(code=%d): %s", authResp.Code, authResp.Desc)
	}

	// Step 3: Follow the location redirect to get serviceToken
	if authResp.Location == "" {
		return nil, fmt.Errorf("认证响应缺少跳转地址")
	}

	req, err = http.NewRequest(http.MethodGet, authResp.Location, nil)
	if err != nil {
		return nil, fmt.Errorf("创建回调请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	// Don't follow redirect - we want to extract the serviceToken from the URL
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // stop at 302
	}
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("回调请求失败: %w", err)
	}
	resp.Body.Close()

	// Reset redirect policy
	client.CheckRedirect = nil

	// Extract serviceToken from the Location header (or from cookies)
	var serviceToken string
	location := resp.Header.Get("Location")
	if location != "" {
		locURL, err := url.Parse(location)
		if err == nil {
			serviceToken = locURL.Query().Get("serviceToken")
		}
	}

	// If serviceToken is empty, try to get from cookies
	if serviceToken == "" {
		for _, c := range jar.Cookies(&url.URL{
			Scheme: "https", Host: "account.xiaomi.com",
		}) {
			if c.Name == "serviceToken" {
				serviceToken = c.Value
				break
			}
		}
	}

	if serviceToken == "" {
		return nil, fmt.Errorf("无法获取 serviceToken，请确认账号密码正确")
	}

	return &xiaomiSession{
		UserID:       authResp.UserID,
		ServiceToken: serviceToken,
		SSecurity:    authResp.SSecurity,
		DeviceID:     generateDeviceID(),
		Client:       client,
	}, nil
}

// ---------------------------------------------------------------------------
// Scale data API (Xiaomi Home `/app/eco/common/scale/getUserDataByPage`)
// ---------------------------------------------------------------------------

func fetchScaleData(sess *xiaomiSession, days int) ([]xiaomiScaleRecord, error) {
	endpoint := "https://api.io.mi.com/app/eco/common/scale/getUserDataByPage"

	now := time.Now()
	from := now.AddDate(0, 0, -days)

	// Build request body
	reqData := map[string]interface{}{
		"limit":   100,
		"offset":  0,
		"from":    from.Unix(),
		"to":      now.Unix(),
		"onlyRaw": true,
	}
	reqJSON, _ := json.Marshal(reqData)
	reqStr := string(reqJSON)

	// Sign the request
	nonce := generateNonce(sess.DeviceID, sess.SSecurity)
	sign := signRequest(sess.SSecurity, nonce, reqStr)
	encryptedData := encryptRC4(sess.SSecurity, reqStr)

	// Build form data
	form := url.Values{}
	form.Set("data", encryptedData)

	respBody, err := sess.callAPI(endpoint, form, nonce, sign)
	if err != nil {
		return nil, err
	}

	var apiResp xiaomiAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}
	if apiResp.Message != "" && apiResp.Result.Code != 0 {
		return nil, fmt.Errorf("API返回错误: %s (code=%d)", apiResp.Message, apiResp.Result.Code)
	}

	return apiResp.Result.Data, nil
}

// callAPI sends a signed POST request to the Xiaomi Home API.
func (sess *xiaomiSession) callAPI(endpoint string, form url.Values, nonce, sign string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建API请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	req.Header.Set("MIOT-REQUEST-MODEL", "yunmai.scales.ms104")

	// Add cookies
	req.AddCookie(&http.Cookie{Name: "userId", Value: sess.UserID})
	req.AddCookie(&http.Cookie{Name: "serviceToken", Value: sess.ServiceToken})

	// Add signed headers
	req.Header.Set("x-xiaomi-protocal-flag-cli", "PROTOCAL-HTTP2")
	req.Header.Set("_nonce", nonce)
	req.Header.Set("signature", sign)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取API响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回状态码 %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}

	return body, nil
}

// ---------------------------------------------------------------------------
// Signing utilities
// ---------------------------------------------------------------------------

// generateNonce creates a Xiaomi-compatible nonce:
//   nonce = base64(SHA1(deviceID + random_bytes))
func generateNonce(deviceID, ssecurity string) string {
	randomBytes := make([]byte, 12)
	_, _ = rand.Read(randomBytes)

	h := sha1.New()
	h.Write([]byte(deviceID))
	h.Write(randomBytes)
	hash := h.Sum(nil)

	return base64.StdEncoding.EncodeToString(hash)
}

// signRequest computes the Xiaomi API signature:
//   sign = base64(HMAC-SHA1(ssecurity, nonce + "&" + data))
func signRequest(ssecurity, nonce, data string) string {
	payload := nonce + "&" + data

	mac := hmac.New(sha1.New, []byte(ssecurity))
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)

	return base64.StdEncoding.EncodeToString(sig)
}

// encryptRC4 encrypts data using RC4 with the ssecurity as key.
// Xiaomi uses this to encrypt request bodies and decrypt responses.
func encryptRC4(ssecurity, data string) string {
	key := sha1Hash(ssecurity)[:16]
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return data // fallback to plaintext
	}
	dst := make([]byte, len(data))
	cipher.XORKeyStream(dst, []byte(data))
	return base64.StdEncoding.EncodeToString(dst)
}

// decryptRC4 decrypts Xiaomi RC4 encrypted data.
func decryptRC4(ssecurity string, encrypted []byte) ([]byte, error) {
	key := sha1Hash(ssecurity)[:16]
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("RC4初始化失败: %w", err)
	}
	dst := make([]byte, len(encrypted))
	cipher.XORKeyStream(dst, encrypted)
	return dst, nil
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------

// recordsToMeasurements converts Xiaomi API records to BodyMeasurement slice.
func recordsToMeasurements(records []xiaomiScaleRecord) ([]models.BodyMeasurement, error) {
	measurements := make([]models.BodyMeasurement, 0, len(records))
	for _, r := range records {
		if r.Weight <= 0 {
			continue // skip invalid entries
		}

		m := models.BodyMeasurement{
			Date:         timestampToDate(r.Timestamp),
			WeightKg:     r.Weight,
			BodyFatPct:   r.BodyFat,
			MuscleMassKg: r.Muscle,
			BoneMassKg:   r.Bone,
			WaterPct:     r.Water,
			BMRKcal:      r.BMR,
			Source:       "xiaomi",
		}
		measurements = append(measurements, m)
	}
	return measurements, nil
}

// timestampToDate converts Unix timestamp to "2006-01-02" date string.
func timestampToDate(ts int64) string {
	return time.Unix(ts, 0).Format("2006-01-02")
}

// ---------------------------------------------------------------------------
// Crypto helpers
// ---------------------------------------------------------------------------

// sha1Upper returns uppercase hex of SHA1(s) — used for password hashing.
func sha1Upper(s string) string {
	h := sha1.Sum([]byte(s))
	return strings.ToUpper(hex.EncodeToString(h[:]))
}

// sha1Hash returns raw SHA1 hash of s.
func sha1Hash(s string) []byte {
	h := sha1.Sum([]byte(s))
	return h[:]
}

// generateDeviceID creates a random device identifier for the API session.
func generateDeviceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ensure strconv is used (for potential future parsing).
var _ = strconv.Itoa
