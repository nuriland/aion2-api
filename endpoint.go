package aion2

type endpoint struct {
	feature Feature
	path    string
	host    host
}

// host is which of NC's backends an endpoint lives on
type host int

const (
	// @TODO: cleanup

	siteHost      host = iota // the game site -- characters and rankings
	gameDataHost              // the game site's servers, classes and items
	searchHost                // character search
	dictHost                  // the item dictionary
	communityHost             // the community boards
	styleshopHost             // the styleshop
)

var (
	serversEndpoint      = endpoint{feature: FeatureServers, path: "/api/gameinfo/servers", host: gameDataHost}
	classesEndpoint      = endpoint{feature: FeatureClasses, path: "/api/gameinfo/classes", host: gameDataHost}
	pcDataEndpoint       = endpoint{feature: FeatureClasses, path: "/api/gameinfo/pcdata", host: gameDataHost}
	searchEndpoint       = endpoint{feature: FeatureSearch, path: "/character", host: searchHost}
	characterEndpoint    = endpoint{feature: FeatureCharacters, path: "/api/character/info"}
	equipmentEndpoint    = endpoint{feature: FeatureCharacters, path: "/api/character/equipment"}
	equippedItemEndpoint = endpoint{feature: FeatureCharacters, path: "/api/character/equipment/item"}
	daevanionEndpoint    = endpoint{feature: FeatureCharacters, path: "/api/character/daevanion/detail"}
	rankingsEndpoint     = endpoint{feature: FeatureRankings, path: "/api/ranking/list"}
	itemEndpoint         = endpoint{feature: FeatureItems, path: "/api/gameconst/item", host: gameDataHost}
	itemsEndpoint        = endpoint{feature: FeatureItemSearch, path: "/dict/search/item", host: dictHost}
	suggestEndpoint      = endpoint{feature: FeatureItemSearch, path: "/dict/search/item/suggest", host: dictHost}
	gradesEndpoint       = endpoint{feature: FeatureItemSearch, path: "/game/item/grade", host: dictHost}
	categoriesEndpoint   = endpoint{feature: FeatureItemSearch, path: "/game/item/category", host: dictHost}
)
