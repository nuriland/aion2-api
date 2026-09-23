package aion2

// Some features are not supported in all regions, therefore we keep a list of features to check against per region.
type Feature string

const (
	FeatureServers    Feature = "servers"
	FeatureClasses    Feature = "classes"
	FeatureCharacters Feature = "characters"
	FeatureSearch     Feature = "search"
	FeatureItems      Feature = "items"
	FeatureRankings   Feature = "rankings"
	FeatureNews       Feature = "news"
)
