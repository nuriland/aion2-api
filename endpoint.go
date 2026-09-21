package aion2

type endpoint struct {
	feature Feature
	path    string
	dict    bool // if true, under the item dictionary's prefix rather than the site's for some reason
}

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
	itemsEndpoint        = endpoint{feature: FeatureItems, path: "/dict/search/item", dict: true}
	gradesEndpoint       = endpoint{feature: FeatureItems, path: "/game/item/grade", dict: true}
)
