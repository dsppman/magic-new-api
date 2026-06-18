package ghostchannel

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const (
	DefaultCount = 3100
	DefaultSeed  = int64(20260618)
	DefaultTag   = "vertex-ai"
)

type Options struct {
	Count           int
	Seed            int64
	Tag             string
	Now             int64
	Models          string
	RandomUsedQuota bool
}

type Stats struct {
	Count        int
	Enabled      int
	AutoDisabled int
}

type weightedString struct {
	Value  string
	Weight int
}

type serviceAccountKey struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
	UniverseDomain          string `json:"universe_domain"`
}

var vertexDefaultRegions = []weightedString{
	{Value: "us", Weight: 26},
	{Value: "global", Weight: 11},
	{Value: "asia-southeast1", Weight: 7},
	{Value: "europe-west1", Weight: 6},
	{Value: "us-east1", Weight: 5},
	{Value: "asia-east1", Weight: 4},
	{Value: "asia-northeast1", Weight: 4},
	{Value: "europe-west4", Weight: 4},
	{Value: "northamerica-northeast1", Weight: 8},
	{Value: "us-central1", Weight: 3},
	{Value: "us-east4", Weight: 4},
	{Value: "us-west1", Weight: 2},
	{Value: "us-west4", Weight: 7},
}

var firstNames = []string{
	"anh", "bao", "binh", "chi", "cong", "cuong", "dat", "duc", "duy", "giang",
	"hai", "hanh", "hieu", "hoa", "hoai", "hoang", "huy", "khanh", "khoa", "lam",
	"lan", "linh", "long", "mai", "minh", "nam", "ngoc", "nhan", "phuc", "quang",
	"son", "tai", "thao", "thanh", "thien", "trang", "tri", "tue", "tuan", "viet",
	"yen", "alice", "brian", "carlos", "daniel", "emily", "grace", "hannah", "jason",
	"kevin", "laura", "michael", "natalie", "olivia", "rachel", "sophia", "steven",
}

var lastNames = []string{
	"nguyen", "tran", "le", "pham", "hoang", "huynh", "vu", "vo", "dang", "bui",
	"do", "phan", "truong", "ngo", "dinh", "ly", "mai", "doan", "cao", "luu",
	"lam", "trinh", "ta", "mac", "benally", "wurth", "garcia", "miller", "davis",
	"brown", "wilson", "thomas", "martin", "clark", "lee", "walker", "hall", "allen",
	"young",
}

var middleNames = []string{"van", "thi", "minh", "quoc", "thanh", "hoang", "duc", "huu", "gia", "nhat", "ngoc"}

var modelSets = [][]string{
	{
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-2.5-pro",
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
		"gemini-3-flash-preview",
		"gemini-3.1-flash-lite-preview",
		"gemini-3.1-pro-preview",
		"gemini-flash-latest",
		"gemini-flash-lite-latest",
		"gemini-3-pro-preview",
	},
	{
		"gemini-2.5-flash",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-lite",
		"gemini-2.5-flash-lite-preview-09-2025",
		"gemini-2.5-pro",
		"gemini-3-pro-image-preview",
		"gemini-3.1-flash-image-preview",
		"gemini-3.1-flash-lite-preview",
		"gemini-3.1-pro-preview",
		"gemini-3.1-pro-preview-customtools",
		"gemini-3-flash-preview",
		"gemini-3-pro-preview",
		"gemini-2.5-flash-native-audio-latest",
	},
}

func Generate(options Options) ([]model.Channel, Stats, error) {
	if options.Count <= 0 {
		options.Count = DefaultCount
	}
	if options.Seed == 0 {
		options.Seed = DefaultSeed
	}
	if options.Tag == "" {
		options.Tag = DefaultTag
	}
	if options.Now == 0 {
		options.Now = time.Now().Unix()
	}
	models := normalizeModels(options.Models)

	rng := rand.New(rand.NewSource(options.Seed))
	statuses := buildStatuses(options.Count, rng)
	emails := make(map[string]struct{}, options.Count)
	settings, err := vertexJSONSettings()
	if err != nil {
		return nil, Stats{}, err
	}

	channels := make([]model.Channel, 0, options.Count)
	stats := Stats{Count: options.Count}
	for i := 0; i < options.Count; i++ {
		status := statuses[i]
		if status == common.ChannelStatusEnabled {
			stats.Enabled++
		} else {
			stats.AutoDisabled++
		}

		email := randomGmail(rng, emails)
		key, err := generateServiceAccountKey(rng, email, i)
		if err != nil {
			return nil, Stats{}, err
		}
		other, err := otherJSON(pickWeightedString(rng, vertexDefaultRegions))
		if err != nil {
			return nil, Stats{}, err
		}
		otherInfo, err := statusInfoJSON(rng, status, options.Now)
		if err != nil {
			return nil, Stats{}, err
		}

		createdAt := options.Now - int64(2*3600+rng.Intn(7*24*3600))
		testAt := options.Now - int64(rng.Intn(36*3600))
		weight := uint(model.GhostChannelMarker)
		priority := int64(model.GhostChannelMarker)
		autoBan := 1

		channels = append(channels, model.Channel{
			Id:                 0,
			Type:               constant.ChannelTypeVertexAi,
			Key:                key,
			Status:             status,
			Name:               email,
			Weight:             &weight,
			CreatedTime:        createdAt,
			TestTime:           testAt,
			ResponseTime:       randomResponseTime(rng, status),
			BaseURL:            nil,
			Other:              other,
			Balance:            0,
			BalanceUpdatedTime: 0,
			Models:             pickModels(rng, models),
			Group:              "Gemini",
			UsedQuota:          usedQuota(rng, options.RandomUsedQuota),
			ModelMapping:       nil,
			StatusCodeMapping:  nil,
			Priority:           &priority,
			AutoBan:            &autoBan,
			OtherInfo:          otherInfo,
			Tag:                common.GetPointer(options.Tag),
			Setting:            nil,
			ParamOverride:      nil,
			HeaderOverride:     nil,
			Remark:             nil,
			ChannelInfo: model.ChannelInfo{
				IsMultiKey:           false,
				MultiKeySize:         0,
				MultiKeyStatusList:   nil,
				MultiKeyPollingIndex: 0,
				MultiKeyMode:         constant.MultiKeyModeRandom,
			},
			OtherSettings: settings,
		})
	}

	return channels, stats, nil
}

func normalizeModels(models string) []string {
	parts := strings.FieldsFunc(models, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		modelName := strings.TrimSpace(part)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
	}
	return result
}

func pickModels(rng *rand.Rand, models []string) string {
	if len(models) > 0 {
		return strings.Join(models, ",")
	}
	return strings.Join(modelSets[rng.Intn(len(modelSets))], ",")
}

func buildStatuses(count int, rng *rand.Rand) []int {
	enabled := int(math.Round(float64(count) * 1020 / 3100))
	statuses := make([]int, count)
	for i := 0; i < count; i++ {
		if i < enabled {
			statuses[i] = common.ChannelStatusEnabled
		} else {
			statuses[i] = common.ChannelStatusAutoDisabled
		}
	}
	rng.Shuffle(len(statuses), func(i, j int) {
		statuses[i], statuses[j] = statuses[j], statuses[i]
	})
	return statuses
}

func vertexJSONSettings() (string, error) {
	bytes, err := common.Marshal(dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeJSON})
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func otherJSON(region string) (string, error) {
	bytes, err := common.Marshal(map[string]string{"default": region})
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func statusInfoJSON(rng *rand.Rand, status int, now int64) (string, error) {
	reason := ""
	if status != common.ChannelStatusEnabled {
		projectNumber := 100000000000 + rng.Int63n(900000000000)
		reasons := []string{
			fmt.Sprintf("Consumer 'project_number:%d' has been suspended. See https://cloud.google.com/billing/docs/how-to/suspended for more information.", projectNumber),
			fmt.Sprintf("status_code=403, bad response status code 403, body: Permission denied: Consumer 'projects/gemini-ent-%06d' has been suspended.", 100000+rng.Intn(900000)),
			"quota exceeded for quota metric 'Generate Content requests' and limit 'Generate content requests per minute'",
			"permission denied while refreshing Vertex AI project credentials",
		}
		reason = reasons[rng.Intn(len(reasons))]
	}
	bytes, err := common.Marshal(map[string]any{
		"status_reason": reason,
		"status_time":   now - int64(rng.Intn(5*24*3600)),
	})
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func randomResponseTime(rng *rand.Rand, status int) int {
	if status != common.ChannelStatusEnabled && rng.Intn(100) < 28 {
		return 0
	}
	base := int(math.Round(math.Exp(math.Log(520) + rng.NormFloat64()*0.45)))
	if base < 12 {
		return 12 + rng.Intn(40)
	}
	if base > 2200 {
		return 1800 + rng.Intn(700)
	}
	return base
}

func randomUsedQuota(rng *rand.Rand) int64 {
	value := int64(math.Round(math.Exp(math.Log(95_000_000) + rng.NormFloat64()*0.78)))
	if rng.Intn(100) < 9 {
		value = int64(800_000 + rng.Intn(12_000_000))
	}
	if rng.Intn(100) < 5 {
		value = int64(210_000_000 + rng.Intn(110_000_000))
	}
	if value < 450_000 {
		return int64(450_000 + rng.Intn(1_800_000))
	}
	if value > 320_000_000 {
		return int64(260_000_000 + rng.Intn(60_000_000))
	}
	return value + int64(rng.Intn(2_500_000))
}

func usedQuota(rng *rand.Rand, random bool) int64 {
	if !random {
		return 0
	}
	return randomUsedQuota(rng)
}

func generateServiceAccountKey(rng *rand.Rand, email string, index int) (string, error) {
	projectNumber := 100000000000 + rng.Int63n(900000000000)
	projectID := fmt.Sprintf("gemini-ent-%06d", 100000+rng.Intn(900000))
	accountName := sanitizeAccountName(email)
	if accountName == "" {
		accountName = "gemini-channel"
	}
	accountName = fmt.Sprintf("%s-%04d", accountName, index%10000)
	clientEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", accountName, projectID)

	key := serviceAccountKey{
		Type:                    "service_account",
		ProjectID:               projectID,
		PrivateKeyID:            randomString(rng, "0123456789abcdef", 40),
		PrivateKey:              randomPrivateKeyPEM(rng),
		ClientEmail:             clientEmail,
		ClientID:                strconv.FormatInt(projectNumber, 10),
		AuthURI:                 "https://accounts.google.com/o/oauth2/auth",
		TokenURI:                "https://oauth2.googleapis.com/token",
		AuthProviderX509CertURL: "https://www.googleapis.com/oauth2/v1/certs",
		ClientX509CertURL:       "https://www.googleapis.com/robot/v1/metadata/x509/" + clientEmail,
		UniverseDomain:          "googleapis.com",
	}
	bytes, err := common.Marshal(key)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func randomGmail(rng *rand.Rand, seen map[string]struct{}) string {
	for tries := 0; tries < 50; tries++ {
		first := firstNames[rng.Intn(len(firstNames))]
		last := lastNames[rng.Intn(len(lastNames))]
		middle := middleNames[rng.Intn(len(middleNames))]
		var local string
		switch roll := rng.Intn(100); {
		case roll < 26:
			local = fmt.Sprintf("%s%s%d", first, last, 10+rng.Intn(9890))
		case roll < 52:
			local = fmt.Sprintf("%s.%s%d", first, last, 100+rng.Intn(900))
		case roll < 70:
			local = fmt.Sprintf("%s%s%d", last, first, 1000+rng.Intn(9000))
		case roll < 86:
			local = fmt.Sprintf("%s%s%s%d", first, middle, last, 10+rng.Intn(90))
		default:
			local = fmt.Sprintf("%s%s.%d", first, last, 1984+rng.Intn(23))
		}
		email := local + "@gmail.com"
		if _, ok := seen[email]; !ok {
			seen[email] = struct{}{}
			return email
		}
	}
	email := fmt.Sprintf("user%d@gmail.com", 100000000+rng.Intn(900000000))
	seen[email] = struct{}{}
	return email
}

func randomPrivateKeyPEM(rng *rand.Rand) string {
	lines := []string{
		randomString(rng, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", 64),
		randomString(rng, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", 64),
		randomString(rng, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", 64),
	}
	return "-----BEGIN PRIVATE KEY-----\n" + strings.Join(lines, "\n") + "\n-----END PRIVATE KEY-----\n"
}

func sanitizeAccountName(email string) string {
	name := email
	if at := strings.IndexByte(name, '@'); at >= 0 {
		name = name[:at]
	}
	name = strings.ToLower(name)
	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if allowed {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if len(result) > 24 {
		result = strings.Trim(result[:24], "-")
	}
	return result
}

func pickWeightedString(rng *rand.Rand, values []weightedString) string {
	total := 0
	for _, value := range values {
		total += value.Weight
	}
	pick := rng.Intn(total)
	for _, value := range values {
		pick -= value.Weight
		if pick < 0 {
			return value.Value
		}
	}
	return values[len(values)-1].Value
}

func randomString(rng *rand.Rand, alphabet string, length int) string {
	var builder strings.Builder
	builder.Grow(length)
	for i := 0; i < length; i++ {
		builder.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return builder.String()
}
