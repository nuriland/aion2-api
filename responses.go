package aion2

import (
	"encoding/json"
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
	Season      *Season           `json:"season"`      // null: NC has no public season running
	RankingList []json.RawMessage `json:"rankingList"` // kept raw for RankingEntry.Raw
}

// GET {dictPrefix}/dict/search/item
type itemsResponse struct {
	Contents   []ItemSummary `json:"contents"`
	Pagination *itemPaging   `json:"pagination"`
}

// searchRow is a search hit. Ref and the class are built from the extra fields.
type searchRow struct {
	CharacterSummary
	CharacterID     string `json:"characterId"` // percent-encoded
	ServerID        int    `json:"serverId"`
	PcID            int    `json:"pcId"`
	ProfileImageURL string `json:"profileImageUrl"` // a path on portraitOrigin
}

// profileRow is a profile. An empty characterId means no such character.
type profileRow struct {
	CharacterProfile
	CharacterID string `json:"characterId"`
	PcID        int    `json:"pcId"`
}

// rankingRow is a board row. Ref is built from characterId and the query's server.
type rankingRow struct {
	RankingEntry
	CharacterID string `json:"characterId"` // percent-encoded, like search results
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

// GET {communityURL}/board/{alias}/article
type postsResponse struct {
	ContentList []postRow `json:"contentList"`
}

// GET {communityURL}/board/{alias}/article/{id}
type postResponse struct {
	Article *articleRow `json:"article"` // null: no such post
}

// GET {communityURL}/board/{alias}
type boardResponse struct {
	Board *struct {
		Name string `json:"boardName"`
	} `json:"board"` // null: no such board
}

// GET {communityURL}/board/{alias}/noticeArticle
type pinnedResponse struct {
	NoticesList []struct {
		ArticleMeta postRow `json:"articleMeta"`
	} `json:"noticesList"`
}

// GET {communityURL}/board/{alias}/article/{id}/comment/search/moreComment
type commentsResponse struct {
	ContentList []articleRow `json:"contentList"`
}

// articleRow is a post or a comment with its body. A comment is a post row without a title.
type articleRow struct {
	ContentMeta postRow `json:"contentMeta"`
	Content     struct {
		Body string `json:"content"`
	} `json:"content"`
}

// postRow is a post as the community API lists it: nested where Post is flat.
type postRow struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	ThumbnailURL string `json:"thumbnailUrl"`
	Timestamps   struct {
		PostedEpoch  int64 `json:"postedEpoch"`
		UpdatedEpoch int64 `json:"updatedEpoch"`
	} `json:"timestamps"`
	Writer struct {
		LoginUser struct {
			Admin bool `json:"admin"`
		} `json:"loginUser"`
		GameUser struct {
			ServerID      string `json:"gameServerId"`    // a string here, an int everywhere else
			CharacterID   string `json:"gameCharacterId"` // percent-encoded, like search results
			CharacterName string `json:"gameCharacterName"`
		} `json:"gameUser"`
	} `json:"writer"`
	Reactions struct {
		ViewCount    int `json:"viewCount"`
		CommentCount int `json:"commentCount"`
	} `json:"reactions"`
}

// GET {styleshopURL}/top100/{site}/ and /search/{site}/
type stylesResponse struct {
	ContentList []styleRow `json:"contentList"`
	HasMore     bool       `json:"hasMore"`
	SearchCount int        `json:"searchCount"` // every match on a search; on the top list, the rows served
}

// GET {styleshopURL}/board/{site}/article/{id}
type styleResponse struct {
	Article *struct {
		ContentMeta postRow `json:"contentMeta"`
		Content     struct {
			Body            string    `json:"content"`
			Tags            []string  `json:"tags"`
			ServiceReserved imageList `json:"serviceReserved"`
		} `json:"content"`
		EquippedItems []StyleSlot `json:"equippedItems"`
		Pet           *StyleItem  `json:"petInfo"`
		Wing          *StyleItem  `json:"wingInfo"`
		Styleshop     styleCounts `json:"styleshop"`
	} `json:"article"` // missing: no such post
}

// styleRow is a post row with the styleshop's counts and screenshots beside it
type styleRow struct {
	postRow
	Styleshop      styleCounts `json:"styleshop"`
	ReservedFields imageList   `json:"reservedFields"`
}

type styleCounts struct {
	LikeCount     int `json:"likeCount"`
	DownloadCount int `json:"downloadCount"`
}

// imageList is where the styleshop keeps a post's screenshots: a JSON array inside a JSON string
type imageList struct {
	Reserved3 struct {
		Contents struct {
			JSON string `json:"json"`
		} `json:"contents"`
	} `json:"reserved3"`
}

func (l imageList) urls() ([]string, error) {
	if l.Reserved3.Contents.JSON == "" {
		return nil, nil
	}
	var urls []string
	err := json.Unmarshal([]byte(l.Reserved3.Contents.JSON), &urls)
	return urls, err
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
}

// highlight is the markup search wraps around the part of a name that matched.
var highlight = strings.NewReplacer("<strong>", "", "</strong>", "")
