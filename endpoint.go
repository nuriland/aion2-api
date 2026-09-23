package aion2

type endpoint struct {
	feature Feature
	path    string
	host    host
}

// host is which of NC's backends an endpoint lives on
type host int

const (
	siteHost      host = iota // the game site -- characters, servers, and classes
	dictHost                  // the item dictionary
	communityHost             // the community boards
)

var (
	serversEndpoint      = endpoint{feature: FeatureServers, path: "/api/gameinfo/servers"}
	classesEndpoint      = endpoint{feature: FeatureClasses, path: "/api/gameinfo/classes"}
	pcDataEndpoint       = endpoint{feature: FeatureClasses, path: "/api/gameinfo/pcdata"}
	searchEndpoint       = endpoint{feature: FeatureSearch, path: "/api/search/character"}
	characterEndpoint    = endpoint{feature: FeatureCharacters, path: "/api/character/info"}
	equipmentEndpoint    = endpoint{feature: FeatureCharacters, path: "/api/character/equipment"}
	equippedItemEndpoint = endpoint{feature: FeatureCharacters, path: "/api/character/equipment/item"}
	daevanionEndpoint    = endpoint{feature: FeatureCharacters, path: "/api/character/daevanion/detail"}
	rankingsEndpoint     = endpoint{feature: FeatureRankings, path: "/api/ranking/list"}
	itemsEndpoint        = endpoint{feature: FeatureItems, path: "/dict/search/item", host: dictHost}
	gradesEndpoint       = endpoint{feature: FeatureItems, path: "/game/item/grade", host: dictHost}
	categoriesEndpoint   = endpoint{feature: FeatureItems, path: "/game/item/category", host: dictHost}
)
