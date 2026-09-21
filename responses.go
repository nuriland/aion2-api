package aion2

import (
	"encoding/json"
	"math"
	"strings"
)

// What NC sends that is not itself a public type, e.g. the envelope around each response

// GET /api/gameinfo/servers
type serversResponse struct {
	ServerList []Server `json:"serverList"`
}

// GET /api/gameinfo/classes
type classesResponse struct {
	ClassList []Class `json:"classList"`
}

// GET /api/gameinfo/pcdata
type pcDataResponse struct {
	PcDataList []pcRow `json:"pcDataList"`
}

// GET /api/search/character
type searchResponse struct {
	List       []searchRow   `json:"list"`
	Pagination *searchPaging `json:"pagination"`
}

// GET /api/character/info
type characterResponse struct {
	Profile *profileRow `json:"profile"`
	Stat    struct {
		StatList []Stat `json:"statList"`
	} `json:"stat"`
	Title   TitleSummary `json:"title"`
	Ranking struct {
		RankingList []CharacterRanking `json:"rankingList"`
	} `json:"ranking"`
	Daevanion struct {
		BoardList []DaevanionSummary `json:"boardList"`
	} `json:"daevanion"`
}

// GET /api/character/equipment
type equipmentResponse struct {
	Equipment *struct {
		EquipmentList []EquipSlot `json:"equipmentList"`
		SkinList      []EquipSlot `json:"skinList"`
	} `json:"equipment"`
	Petwing struct {
		Pet      *Pet  `json:"pet"`
		Wing     *Wing `json:"wing"`
		WingSkin *Wing `json:"wingSkin"`
	} `json:"petwing"`
	Skill struct {
		SkillList []Skill `json:"skillList"`
	} `json:"skill"`
}

// GET /api/ranking/list
type rankingsResponse struct {
	RankingList []json.RawMessage `json:"rankingList"` // kept raw for RankingEntry.Raw
	Season      *Season           `json:"season"`
}

// GET {dictPrefix}/dict/search/item
type itemsResponse struct {
	Contents   []json.RawMessage `json:"contents"` // kept raw for Item.Raw
	Pagination *itemPaging       `json:"pagination"`
}

// GET {dictPrefix}/game/item/grade
type gradeRow struct {
	ID string `json:"id"`
}

// searchRow is a search hit. Ref and the class are built from the extra fields.
type searchRow struct {
	CharacterSummary
	CharacterID string `json:"characterId"` // percent-encoded
	ServerID    int    `json:"serverId"`
	PcID        int    `json:"pcId"`
}

// profileRow is a profile. An empty characterId means no such character.
type profileRow struct {
	CharacterProfile
	CharacterID string `json:"characterId"`
	PcID        int    `json:"pcId"`
}

type rankingRow struct {
	RankingEntry
	CharacterID string `json:"characterId"`
}

// pcRow is one class x race x gender combination.
type pcRow struct {
	ID        int    `json:"id"`
	ClassName string `json:"className"` // Class.Name, upper-cased
}

type searchPaging struct {
	Page    int `json:"page"`
	Size    int `json:"size"`
	Total   int `json:"total"`
	EndPage int `json:"endPage"`
}

// descRow is one entry of a Lines list as NC sends it.
type descRow struct {
	Desc string `json:"desc"`
}

type itemPaging struct {
	Page     int `json:"page"`
	Size     int `json:"size"`
	LastPage int `json:"lastPage"`
	Total    int `json:"total"`
	Limit    int `json:"limit"` // upstream refuses to page past this many rows
}

// highlight is the markup search wraps around the part of a name that matched.
var highlight = strings.NewReplacer("<strong>", "", "</strong>", "")

// newOnBoard is what NC puts in rankChange for a character with no previous rank.
const newOnBoard = math.MaxInt32
