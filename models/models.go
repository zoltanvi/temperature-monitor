package models

type TempPoint struct {
	Timestamp int64   `json:"ts"`
	Value     float64 `json:"v"`
}

type AggregatedTempPoint struct {
	Timestamp int64   `json:"ts"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Mean      float64 `json:"mean"`
	Count     float64 `json:"n"`
}

// RollupBucket is a pre-aggregated time bucket stored in SQLite.
// Mean is derived as Sum/Count; do not persist mean.
type RollupBucket struct {
	BucketStart int64
	Min         float64
	Max         float64
	Sum         float64
	Count       int64
}

type Statistics struct {
	StartTimestamp          int64   `json:"start"`
	EndTimestamp            int64   `json:"end"`
	CurrentTemp             float64 `json:"currentTemp"`
	Min                     float64 `json:"min"`
	Max                     float64 `json:"max"`
	Mean                    float64 `json:"mean"`
	TimeInWarning           float64 `json:"timeInWarning"`
	TimeInCritical          float64 `json:"timeInCritical"`
	RequestElapsedTime      string  `json:"requestElapsedTime"`
	RequestProcessedSamples string  `json:"requestProcessedSamples"`
	StatsApproximate        bool    `json:"statsApproximate"`
}

// ResponseMeta carries request diagnostics for the UI status line.
type ResponseMeta struct {
	NoData              bool   `json:"noData"`
	DataSource          string `json:"dataSource"`
	ProcessedSamples    int    `json:"processedSamples"`
	LastSampleAgeSec    *int64 `json:"lastSampleAgeSec"`
	DBSizeBytes         int64  `json:"dbSizeBytes"`
}

type TemperatureApiResponse struct {
	Statistics     Statistics            `json:"statistics"`
	AggregatedData []AggregatedTempPoint `json:"data"`
	Meta           ResponseMeta          `json:"meta"`
}

// HealthResponse is returned by GET /api/v3/health.
type HealthResponse struct {
	OK               bool   `json:"ok"`
	UptimeSec        int64  `json:"uptimeSec"`
	RollupsReady     bool   `json:"rollupsReady"`
	LastSampleAgeSec *int64 `json:"lastSampleAgeSec"`
	LastSampleTs     *int64 `json:"lastSampleTs"`
	DBSizeBytes      int64  `json:"dbSizeBytes"`
}
