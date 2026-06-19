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
	Count                  int
	Seed                   int64
	Tag                    string
	Now                    int64
	Models                 string
	Group                  string
	Groups                 []string
	RandomUsedQuota        bool
	RandomAutoDisable      *bool
	RandomDisableStartTime int64
	RandomDisableEndTime   int64
	RandomResponseTime     bool
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

type weightedModelSet struct {
	Models []string
	Weight int
}

type poolTimelineEntry struct {
	CreatedTime int64
	Status      int
	StatusTime  int64
	TestTime    int64
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

var channelNamePrefixes = []weightedString{
	{Value: "vertex", Weight: 24},
	{Value: "vertex-ai", Weight: 20},
	{Value: "gemini", Weight: 18},
	{Value: "gcp-vertex", Weight: 12},
	{Value: "genai", Weight: 10},
	{Value: "ai-platform", Weight: 8},
	{Value: "google-ai", Weight: 8},
}

var channelNameRoles = []weightedString{
	{Value: "chat", Weight: 22},
	{Value: "gateway", Weight: 18},
	{Value: "pool", Weight: 14},
	{Value: "shared", Weight: 12},
	{Value: "prod", Weight: 12},
	{Value: "svc", Weight: 10},
	{Value: "vision", Weight: 6},
	{Value: "batch", Weight: 4},
	{Value: "embed", Weight: 2},
}

var projectPrefixes = []weightedString{
	{Value: "gemini-prod", Weight: 22},
	{Value: "vertex-prod", Weight: 20},
	{Value: "genai-platform", Weight: 16},
	{Value: "gcp-ai", Weight: 14},
	{Value: "llm-prod", Weight: 12},
	{Value: "ml-serving", Weight: 10},
	{Value: "aiplatform", Weight: 6},
}

var projectWords = []weightedString{
	{Value: "core", Weight: 16},
	{Value: "chat", Weight: 14},
	{Value: "gateway", Weight: 13},
	{Value: "prod", Weight: 12},
	{Value: "shared", Weight: 10},
	{Value: "vision", Weight: 8},
	{Value: "serve", Weight: 8},
	{Value: "batch", Weight: 6},
	{Value: "embed", Weight: 5},
	{Value: "ops", Weight: 4},
}

var defaultModelSets = []weightedModelSet{
	{
		Models: []string{"gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-2.5-pro"},
		Weight: 24,
	},
	{
		Models: []string{"gemini-2.5-flash", "gemini-2.0-flash", "gemini-2.0-flash-lite", "gemini-2.5-pro"},
		Weight: 18,
	},
	{
		Models: []string{"gemini-2.5-flash", "gemini-2.5-flash-lite", "text-embedding-004", "gemini-embedding-001"},
		Weight: 14,
	},
	{
		Models: []string{"gemini-2.5-flash", "gemini-2.5-pro", "gemini-2.5-flash-image", "imagen-4.0-generate-001"},
		Weight: 12,
	},
	{
		Models: []string{"gemini-2.5-flash", "imagen-4.0-generate-001", "imagen-4.0-fast-generate-001"},
		Weight: 9,
	},
	{
		Models: []string{"gemini-2.5-flash", "gemini-2.5-pro", "veo-3.0-generate-001", "veo-3.0-fast-generate-001"},
		Weight: 7,
	},
	{
		Models: []string{"gemini-flash-latest", "gemini-flash-lite-latest", "gemini-2.5-flash"},
		Weight: 10,
	},
	{
		Models: []string{"gemini-3-pro-preview", "gemini-3-flash-preview", "gemini-2.5-flash"},
		Weight: 6,
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
	group := normalizeGroup(options.Group, options.Groups)
	randomAutoDisable := options.RandomUsedQuota
	if options.RandomAutoDisable != nil {
		randomAutoDisable = *options.RandomAutoDisable
	}

	rng := rand.New(rand.NewSource(options.Seed))
	timeline := buildPoolTimeline(options.Count, rng, options.Now, randomAutoDisable, options.RandomDisableStartTime, options.RandomDisableEndTime)
	names := make(map[string]struct{}, options.Count)
	settings, err := vertexJSONSettings()
	if err != nil {
		return nil, Stats{}, err
	}

	channels := make([]model.Channel, 0, options.Count)
	stats := Stats{Count: options.Count}
	for i := 0; i < options.Count; i++ {
		entry := timeline[i]
		status := entry.Status
		if status == common.ChannelStatusEnabled {
			stats.Enabled++
		} else {
			stats.AutoDisabled++
		}

		region := pickWeightedString(rng, vertexDefaultRegions)
		name := randomChannelName(rng, names, region, i)
		key, err := generateServiceAccountKey(rng, name, i)
		if err != nil {
			return nil, Stats{}, err
		}
		other, err := otherJSON(region)
		if err != nil {
			return nil, Stats{}, err
		}
		otherInfo, err := statusInfoJSON(rng, status, entry.StatusTime)
		if err != nil {
			return nil, Stats{}, err
		}

		weight := uint(model.GhostChannelMarker)
		priority := int64(model.GhostChannelMarker)
		autoBan := 1

		channels = append(channels, model.Channel{
			Id:                 0,
			Type:               constant.ChannelTypeVertexAi,
			Key:                key,
			Status:             status,
			Name:               name,
			Weight:             &weight,
			CreatedTime:        entry.CreatedTime,
			TestTime:           entry.TestTime,
			ResponseTime:       responseTime(rng, status, options.RandomResponseTime),
			BaseURL:            nil,
			Other:              other,
			Balance:            0,
			BalanceUpdatedTime: 0,
			Models:             pickModels(rng, models),
			Group:              group,
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

func normalizeGroup(group string, groups []string) string {
	parts := make([]string, 0, len(groups)+1)
	if len(groups) > 0 {
		parts = append(parts, groups...)
	} else {
		parts = strings.FieldsFunc(group, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t'
		})
	}

	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		groupName := strings.TrimSpace(part)
		if groupName == "" {
			continue
		}
		if _, ok := seen[groupName]; ok {
			continue
		}
		seen[groupName] = struct{}{}
		result = append(result, groupName)
	}
	if len(result) == 0 {
		return "Gemini"
	}
	return strings.Join(result, ",")
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
		return strings.Join(pickProvidedModelSubset(rng, models), ",")
	}
	return strings.Join(pickDefaultModelSet(rng), ",")
}

func pickDefaultModelSet(rng *rand.Rand) []string {
	total := 0
	for _, modelSet := range defaultModelSets {
		total += modelSet.Weight
	}
	pick := rng.Intn(total)
	for _, modelSet := range defaultModelSets {
		pick -= modelSet.Weight
		if pick < 0 {
			return append([]string(nil), modelSet.Models...)
		}
	}
	return append([]string(nil), defaultModelSets[len(defaultModelSets)-1].Models...)
}

func pickProvidedModelSubset(rng *rand.Rand, models []string) []string {
	if len(models) <= 2 {
		return append([]string(nil), models...)
	}
	targetSize := 3 + rng.Intn(min(4, len(models)-2))
	if targetSize > len(models) {
		targetSize = len(models)
	}
	indexes := rng.Perm(len(models))[:targetSize]
	seen := map[int]struct{}{}
	for _, index := range indexes {
		seen[index] = struct{}{}
	}
	result := make([]string, 0, targetSize)
	for i, modelName := range models {
		if _, ok := seen[i]; ok {
			result = append(result, modelName)
		}
	}
	if !common.StringsContains(result, "gemini-2.5-flash") && common.StringsContains(models, "gemini-2.5-flash") && len(result) > 0 {
		result[0] = "gemini-2.5-flash"
	}
	return result
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

func buildPoolTimeline(count int, rng *rand.Rand, now int64, randomAutoDisable bool, randomDisableStartTime int64, randomDisableEndTime int64) []poolTimelineEntry {
	createdTimes := sequentialRandomCreatedTimes(rng, count, now)
	entries := make([]poolTimelineEntry, count)
	for i := 0; i < count; i++ {
		entries[i] = poolTimelineEntry{
			CreatedTime: createdTimes[i],
			Status:      common.ChannelStatusEnabled,
			StatusTime:  enabledStatusTime(rng, now, createdTimes[i]),
		}
	}

	if randomAutoDisable {
		assignAutoDisabledTimeline(entries, rng, now, randomDisableStartTime, randomDisableEndTime)
	}

	for i := range entries {
		entries[i].TestTime = randomTestTime(rng, now, entries[i].CreatedTime, entries[i].Status, entries[i].StatusTime)
	}
	return entries
}

func sequentialRandomCreatedTimes(rng *rand.Rand, count int, now int64) []int64 {
	if count <= 0 {
		return nil
	}

	historyDays := 45 + rng.Intn(116)
	start := now - int64(historyDays*24*3600+rng.Intn(18*3600))
	end := now - int64(10*60+rng.Intn(18*3600))
	if end <= start {
		end = start + int64(count)
	}

	return sequentialRandomTimes(rng, count, start, end)
}

func enabledStatusTime(rng *rand.Rand, now int64, createdAt int64) int64 {
	statusTime := now - int64(rng.Intn(5*24*3600))
	if statusTime < createdAt {
		statusTime = createdAt + int64(rng.Intn(3600))
	}
	if statusTime > now {
		return now
	}
	return statusTime
}

func assignAutoDisabledTimeline(entries []poolTimelineEntry, rng *rand.Rand, now int64, randomDisableStartTime int64, randomDisableEndTime int64) {
	_, disableEnd := disableTimelineRange(now, randomDisableStartTime, randomDisableEndTime)
	candidates := make([]int, 0, len(entries)/2)
	for i := range entries {
		if entries[i].CreatedTime+30*60 > disableEnd {
			continue
		}
		if shouldAutoDisableEntry(rng, entries[i].CreatedTime, disableEnd, i, len(entries)) {
			candidates = append(candidates, i)
		}
	}

	if len(candidates) == 0 {
		if index, ok := randomEligibleAutoDisableIndex(entries, rng, disableEnd); ok {
			candidates = append(candidates, index)
		}
	}

	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	statusTimes := SequentialStatusTimes(len(candidates), rng, now, randomDisableStartTime, randomDisableEndTime)
	for i, index := range candidates {
		statusTime := statusTimes[i]
		minStatusTime := entries[index].CreatedTime + 30*60
		if statusTime < minStatusTime {
			statusTime = minStatusTime
		}
		entries[index].StatusTime = statusTime
		entries[index].Status = common.ChannelStatusAutoDisabled
	}
}

func randomEligibleAutoDisableIndex(entries []poolTimelineEntry, rng *rand.Rand, disableEnd int64) (int, bool) {
	eligible := make([]int, 0, len(entries))
	for i := 0; i < len(entries); i++ {
		if entries[i].CreatedTime+30*60 <= disableEnd {
			eligible = append(eligible, i)
		}
	}
	if len(eligible) == 0 {
		return 0, false
	}
	return eligible[rng.Intn(len(eligible))], true
}

func shouldAutoDisableEntry(rng *rand.Rand, createdAt int64, statusTime int64, index int, count int) bool {
	ageDays := int((statusTime - createdAt) / (24 * 3600))
	probability := 32
	switch {
	case ageDays >= 90:
		probability += 24
	case ageDays >= 60:
		probability += 18
	case ageDays >= 30:
		probability += 12
	case ageDays >= 14:
		probability += 6
	}
	if count > 0 && index < count/5 {
		probability += 4
	}
	if probability > 68 {
		probability = 68
	}
	return rng.Intn(100) < probability
}

func SequentialStatusTimes(count int, rng *rand.Rand, now int64, randomDisableStartTime int64, randomDisableEndTime int64) []int64 {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	start, end := disableTimelineRange(now, randomDisableStartTime, randomDisableEndTime)
	return sequentialRandomTimes(rng, count, start, end)
}

func disableTimelineRange(now int64, randomDisableStartTime int64, randomDisableEndTime int64) (int64, int64) {
	if randomDisableStartTime > 0 && randomDisableEndTime >= randomDisableStartTime {
		return randomDisableStartTime, randomDisableEndTime
	}
	return now - 5*24*3600, now
}

func sequentialRandomTimes(rng *rand.Rand, count int, start int64, end int64) []int64 {
	if count <= 0 {
		return nil
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if end <= start {
		end = start + int64(count)
	}
	if count == 1 {
		if end == start {
			return []int64{start}
		}
		return []int64{start + (end-start)/2}
	}
	if end-start < int64(count-1) {
		end = start + int64(count-1)
	}

	weights := make([]float64, count-1)
	total := 0.0
	for i := range weights {
		weight := math.Exp(rng.NormFloat64() * 0.9)
		if rng.Intn(100) < 6 {
			weight *= 2.5 + rng.Float64()*4
		}
		weights[i] = weight
		total += weight
	}

	times := make([]int64, count)
	times[0] = start
	previous := start
	cumulative := 0.0
	span := float64(end - start)
	for i := 1; i < count; i++ {
		cumulative += weights[i-1]
		next := start + int64(math.Round(span*cumulative/total))
		if next <= previous {
			next = previous + 1
		}
		latest := end - int64(count-1-i)
		if next > latest {
			next = latest
		}
		times[i] = next
		previous = next
	}
	times[count-1] = end
	return times
}

func statusInfoJSON(rng *rand.Rand, status int, statusTime int64) (string, error) {
	reason := ""
	if status != common.ChannelStatusEnabled {
		reason = RandomStatusReason(rng)
	}
	bytes, err := common.Marshal(map[string]any{
		"status_reason": reason,
		"status_time":   statusTime,
	})
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func RandomStatusTime(rng *rand.Rand, now int64, randomDisableStartTime int64, randomDisableEndTime int64) int64 {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if randomDisableStartTime > 0 && randomDisableEndTime >= randomDisableStartTime {
		if randomDisableEndTime == randomDisableStartTime {
			return randomDisableStartTime
		}
		return randomDisableStartTime + rng.Int63n(randomDisableEndTime-randomDisableStartTime+1)
	}
	return now - int64(rng.Intn(5*24*3600))
}

func RandomStatusReason(rng *rand.Rand) string {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	projectNumber := 100000000000 + rng.Int63n(900000000000)
	reasons := []string{
		fmt.Sprintf("Consumer 'project_number:%d' has been suspended. See https://cloud.google.com/billing/docs/how-to/suspended for more information.", projectNumber),
		fmt.Sprintf("status_code=403, bad response status code 403, body: Permission denied: Consumer 'projects/gemini-ent-%06d' has been suspended.", 100000+rng.Intn(900000)),
		"quota exceeded for quota metric 'Generate Content requests' and limit 'Generate content requests per minute'",
		"permission denied while refreshing Vertex AI project credentials",
	}
	return reasons[rng.Intn(len(reasons))]
}

func responseTime(rng *rand.Rand, status int, random bool) int {
	if !random {
		return stableResponseTime(rng, status)
	}
	return randomResponseTime(rng, status)
}

func stableResponseTime(rng *rand.Rand, status int) int {
	if status != common.ChannelStatusEnabled && rng.Intn(100) < 40 {
		return 0
	}
	base := 420 + rng.Intn(520)
	if rng.Intn(100) < 8 {
		base += 500 + rng.Intn(900)
	}
	return base
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
		return baselineUsedQuota(rng)
	}
	return randomUsedQuota(rng)
}

func baselineUsedQuota(rng *rand.Rand) int64 {
	if rng.Intn(100) < 12 {
		return int64(rng.Intn(700_000))
	}
	value := int64(math.Round(math.Exp(math.Log(18_000_000) + rng.NormFloat64()*0.9)))
	if value < 250_000 {
		return int64(250_000 + rng.Intn(1_000_000))
	}
	if value > 180_000_000 {
		return int64(120_000_000 + rng.Intn(60_000_000))
	}
	return value + int64(rng.Intn(900_000))
}

func generateServiceAccountKey(rng *rand.Rand, channelName string, index int) (string, error) {
	projectNumber := 100000000000 + rng.Int63n(900000000000)
	projectID := randomProjectID(rng)
	accountName := sanitizeAccountName(channelName)
	if accountName == "" {
		accountName = "vertex-channel"
	}
	if rng.Intn(100) < 70 {
		accountName = fmt.Sprintf("%s-%04d", accountName, index%10000)
	}
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

func randomProjectID(rng *rand.Rand) string {
	prefix := pickWeightedString(rng, projectPrefixes)
	word := pickWeightedString(rng, projectWords)
	number := 1000 + rng.Intn(900000)
	switch rng.Intn(4) {
	case 0:
		return fmt.Sprintf("%s-%06d", prefix, number)
	case 1:
		return fmt.Sprintf("%s-%s-%04d", prefix, word, number%10000)
	case 2:
		return fmt.Sprintf("%s-%s", prefix, randomString(rng, "abcdefghijklmnopqrstuvwxyz0123456789", 6))
	default:
		return fmt.Sprintf("%s-%s-%05d", word, prefix, number%100000)
	}
}

func randomChannelName(rng *rand.Rand, seen map[string]struct{}, region string, index int) string {
	for tries := 0; tries < 50; tries++ {
		prefix := pickWeightedString(rng, channelNamePrefixes)
		role := pickWeightedString(rng, channelNameRoles)
		env := pickWeightedString(rng, []weightedString{
			{Value: "prod", Weight: 48},
			{Value: "prd", Weight: 18},
			{Value: "shared", Weight: 14},
			{Value: "stable", Weight: 10},
			{Value: "online", Weight: 6},
			{Value: "canary", Weight: 4},
		})
		regionCode := shortRegion(region)
		randomSuffix := randomString(rng, "abcdefghijklmnopqrstuvwxyz0123456789", 4)
		number := 1 + rng.Intn(999)

		var name string
		switch rng.Intn(7) {
		case 0:
			name = fmt.Sprintf("%s-%s-%s-%03d", prefix, role, regionCode, number)
		case 1:
			name = fmt.Sprintf("%s-%s-%s", prefix, env, randomSuffix)
		case 2:
			name = fmt.Sprintf("%s-%s-%s-%02d", role, prefix, regionCode, 1+rng.Intn(80))
		case 3:
			name = fmt.Sprintf("%s-%s-%s", prefix, regionCode, randomSuffix)
		case 4:
			name = fmt.Sprintf("%s-%s-%03d", env, role, number)
		case 5:
			name = fmt.Sprintf("%s-%s-%s", prefix, role, randomSuffix)
		default:
			name = fmt.Sprintf("%s-%s-%04d", prefix, role, (index+rng.Intn(7000))%10000)
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			return name
		}
	}
	name := fmt.Sprintf("vertex-prod-%s-%04d", shortRegion(region), index)
	seen[name] = struct{}{}
	return name
}

func shortRegion(region string) string {
	parts := strings.Split(region, "-")
	if len(parts) == 0 || parts[0] == "" {
		return "global"
	}
	if region == "global" {
		return "global"
	}
	if len(parts) == 1 {
		return parts[0]
	}
	last := parts[len(parts)-1]
	if len(last) > 1 {
		last = last[len(last)-1:]
	}
	return strings.Join(parts[:len(parts)-1], "") + last
}

func randomTestTime(rng *rand.Rand, now int64, createdAt int64, status int, statusTime int64) int64 {
	var testAt int64
	if status != common.ChannelStatusEnabled {
		offset := int64(rng.Intn(12 * 3600))
		if statusTime > offset {
			testAt = statusTime - offset
		} else {
			testAt = statusTime
		}
	} else {
		ageSeconds := int64(math.Round(math.Exp(math.Log(8*3600) + rng.NormFloat64()*1.05)))
		if ageSeconds < 5*60 {
			ageSeconds = int64(5*60 + rng.Intn(55*60))
		}
		if ageSeconds > 12*24*3600 {
			ageSeconds = int64(2*24*3600 + rng.Intn(10*24*3600))
		}
		testAt = now - ageSeconds
	}
	if testAt < createdAt {
		testAt = createdAt + int64(rng.Intn(24*3600))
	}
	if testAt > now {
		if now <= createdAt {
			return createdAt
		}
		testAt = createdAt + rng.Int63n(now-createdAt+1)
	}
	if testAt < createdAt {
		testAt = createdAt
	}
	return testAt
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
