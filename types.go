package aion2

import (
	"encoding/json"
	"fmt"
	"time"
)

// Locale changes labels, never the backend.
// LocaleEN on KR still returns KR servers with Korean names.
type Locale string

const (
	LocaleKO   Locale = "ko"    // Korean
	LocaleZHTW Locale = "zh-TW" // Taiwanese
	LocaleEN   Locale = "en"    // English
)

// Region is the region of the AION 2 website, not the language
type Region string

const (
	RegionKR     Region = "kr"
	RegionTW     Region = "tw"
	RegionGlobal Region = "global" // reserved
)

// Server is one world. IDs collide across regions
type Server struct {
	Region    Region `json:"region"`
	ServerID  int    `json:"serverId"`
	RaceID    int    `json:"raceId"` // 1 = Ely, 2 = Asmo
	Name      string `json:"serverName"`
	ShortName string `json:"serverShortName"`
}

type Class struct {
	ID   int    `json:"id"`
	Name string `json:"name"` // English slug: Gladiator, Templar, ...
	Text string `json:"text"` // localized: 검성 / 劍星 / Gladiator
}

// CharacterRef identifies a character within one region
type CharacterRef struct {
	ServerID    int    `json:"serverId"`
	CharacterID string `json:"characterId"`
}

type Character struct {
	Region    Region             `json:"region"`
	Profile   CharacterProfile   `json:"profile"`
	Stats     []Stat             `json:"stats"`
	Titles    TitleSummary       `json:"titles"`
	Rankings  []CharacterRanking `json:"rankings"`
	Daevanion []DaevanionSummary `json:"daevanion"`
}

type CharacterProfile struct {
	Ref         CharacterRef `json:"ref"`
	Name        string       `json:"characterName"`
	Level       int          `json:"characterLevel"`
	ClassID     int          `json:"classId"`    // 0 if NC's class table was unreachable or does not list the class
	ClassName   string       `json:"className"`  // localized, from the profile itself; set even when ClassID is 0
	Gender      string       `json:"genderName"` // localized
	RaceID      int          `json:"raceId"`
	ServerName  string       `json:"serverName"`
	GuildName   string       `json:"regionName"` // NC files the guild under regionName; its own schema calls it 길드 명. No guild id is sent
	CombatPower int          `json:"combatPower"`
	TitleName   string       `json:"titleName"`
	ImageURL    string       `json:"profileImage"`
}

type Stat struct {
	Type    string   `json:"type"` // STR, DEX, ..., ItemLevel
	Name    string   `json:"name"` // localized
	Value   int      `json:"value"`
	Effects []string `json:"statSecondList"`
}

type TitleSummary struct {
	Total  int     `json:"totalCount"`
	Owned  int     `json:"ownedCount"`
	Titles []Title `json:"titleList"`
}

type Title struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Grade        string `json:"grade"`
	Category     string `json:"equipCategory"`
	Total        int    `json:"totalCount"`
	Owned        int    `json:"ownedCount"`
	OwnedPercent int    `json:"ownedPercent"`
	Stats        Lines  `json:"statList"`
	EquipStats   Lines  `json:"equipStatList"`
}

type CharacterRanking struct {
	ContentsType int     `json:"rankingContentsType"`
	ContentsName string  `json:"rankingContentsName"`
	RankingType  int     `json:"rankingType"`
	Rank         int     `json:"rank"`
	PrevRank     int     `json:"prevRank"`
	RankChange   int     `json:"rankChange"`
	Point        float64 `json:"point"`
	GradeName    string  `json:"gradeName"`
	GradeIconURL string  `json:"gradeIcon"`
}

// DaevanionSummary is one board's progress. Its ID opens the board itself
// through Daevanion.
type DaevanionSummary struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	IconURL     string `json:"icon"`
	Open        Flag   `json:"open"`
	OpenNodes   int    `json:"openNodeCount"`
	TotalNodes  int    `json:"totalNodeCount"`
	OpenPercent int    `json:"openPercent"`
}

type CharacterSearch struct {
	Keyword  string // required
	RaceID   int    // required, 1 = Ely, 2 = Asmo
	ServerID int    // 0 = every server in the region
	ClassIDs []int  // Class.ID values
	Sort     string
	Page     int // default 1
	Size     int // default 40; 200 works
}

type CharacterSummary struct {
	Ref        CharacterRef `json:"ref"`
	Region     Region       `json:"region"`
	Name       string       `json:"name"`
	ServerName string       `json:"serverName"`
	ClassID    int          `json:"classId"`   // 0 if NC's class table was unreachable or does not list the class
	ClassName  string       `json:"className"` // localized; empty whenever ClassID is 0
	RaceID     int          `json:"race"`
	Level      int          `json:"level"`
	ImageURL   string       `json:"imageUrl"` // the portrait; not every character has one
}

type Equipment struct {
	Region   Region       `json:"region"`
	Ref      CharacterRef `json:"ref"`
	Slots    []EquipSlot  `json:"slots"` // worn gear, Arcana included, slot pos 41 and up, slot name Arcana1, Arcana2, ...
	Skins    []EquipSlot  `json:"skins"`
	Pet      *Pet         `json:"pet"`
	Wing     *Wing        `json:"wing"`
	WingSkin *Wing        `json:"wingSkin"`
	Skills   []Skill      `json:"skills"`
}

type EquipSlot struct {
	SlotPos      int    `json:"slotPos"`
	SlotName     string `json:"slotPosName"`
	ItemID       int    `json:"id"`
	Name         string `json:"name"`
	Grade        string `json:"grade"`
	EnchantLevel int    `json:"enchantLevel"`
	ExceedLevel  int    `json:"exceedLevel"`
	ImageURL     string `json:"icon"`
}

type Pet struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Level    int    `json:"level"`
	ImageURL string `json:"icon"`
}

type Wing struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Grade        string `json:"grade"`
	EnchantLevel int    `json:"enchantLevel"`
	ImageURL     string `json:"icon"`
}

type Skill struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	Level     int    `json:"skillLevel"`
	NeedLevel int    `json:"needLevel"`
	Acquired  Flag   `json:"acquired"`
	Equipped  Flag   `json:"equip"`
	ImageURL  string `json:"icon"`
}

// EquippedItem is one worn piece in full. Stat values stay the strings NC
// sends ("491", "+80", "12.5%"): they are for display, not arithmetic.
type EquippedItem struct {
	Region          Region          `json:"region"`
	Ref             CharacterRef    `json:"ref"`
	SlotPos         int             `json:"slotPos"`
	ItemID          int             `json:"id"`
	Name            string          `json:"name"`
	Grade           string          `json:"grade"`     // Epic
	GradeName       string          `json:"gradeName"` // localized: Heroic
	CategoryName    string          `json:"categoryName"`
	Type            string          `json:"type"` // Equip, Accessory
	ImageURL        string          `json:"icon"`
	ItemLevel       int             `json:"level"`
	EquipLevel      int             `json:"equipLevel"`
	EnchantLevel    int             `json:"enchantLevel"`
	MaxEnchantLevel int             `json:"maxEnchantLevel"`
	MaxExceedLevel  int             `json:"maxExceedEnchantLevel"`
	RaceName        string          `json:"raceName"`
	ClassNames      []string        `json:"classNames"`
	Tradable        bool            `json:"tradable"`
	SoulBindRate    string          `json:"soulBindRate"`
	MainStats       []ItemStat      `json:"mainStats"`
	SubStats        []ItemStat      `json:"subStats"` // the random rolls
	SubSkills       []ItemSkill     `json:"subSkills"`
	MagicStoneSlots int             `json:"magicStoneSlotCount"`
	MagicStones     []MagicStone    `json:"magicStoneStat"`
	GodStoneSlots   int             `json:"godStoneSlotCount"`
	GodStones       []GodStone      `json:"godStoneStat"`
	Costumes        []string        `json:"costumes"`
	Sources         []string        `json:"sources"`
	Raw             json.RawMessage `json:"-"`
}

type ItemStat struct {
	ID       string `json:"id"`   // NC's stat key: ArmorDefense, HPMax, ...
	Name     string `json:"name"` // localized
	Value    string `json:"value"`
	MinValue string `json:"minValue"` // on weapons, the low end of the damage range
	Extra    string `json:"extra"`    // on main stats, the share that comes from enchanting
	Exceed   bool   `json:"exceed"`
}

type ItemSkill struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Level    int    `json:"level"`
	ImageURL string `json:"icon"`
}

type MagicStone struct {
	SlotPos  int    `json:"slotPos"`
	ID       string `json:"id"` // stat key
	Name     string `json:"name"`
	Value    string `json:"value"`
	Grade    string `json:"grade"`
	ImageURL string `json:"icon"`
}

type GodStone struct {
	SlotPos  int    `json:"slotPos"`
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	Grade    string `json:"grade"`
	ImageURL string `json:"icon"`
}

type DaevanionBoard struct {
	Region       Region          `json:"region"`
	Ref          CharacterRef    `json:"ref"`
	BoardID      int             `json:"boardId"`
	Nodes        []DaevanionNode `json:"nodeList"`
	SkillEffects Lines           `json:"openSkillEffectList"`
	StatEffects  Lines           `json:"openStatEffectList"`
}

type DaevanionNode struct {
	ID      int    `json:"nodeId"`
	Name    string `json:"name"`
	Grade   string `json:"grade"`
	Type    string `json:"type"` // Start, Stat, SkillLevel, None
	IconURL string `json:"icon"`
	Row     int    `json:"row"`
	Col     int    `json:"col"`
	Open    Flag   `json:"open"`
	Effects Lines  `json:"effectList"`
}

// ItemSearch filters use the IDs the catalog publishes about itself.
//
// Grade is Common, Rare, Legend, Unique or Epic.
//
// Category is a top-level id such as Equip_Weapon; SubCategory one of its children, such as Greatsword.
type ItemSearch struct {
	Query       string
	Grade       string
	Category    string
	SubCategory string
	ClassID     int // Class.ID
	Page        int // default 1
	Size        int // default 30; 200 works
}

type ItemSummary struct {
	ID           int      `json:"id"`
	Region       Region   `json:"region"` // the catalog it came from, not "exists only here"
	Name         string   `json:"name"`
	Grade        string   `json:"grade"`
	ImageURL     string   `json:"image"`
	CategoryName string   `json:"categoryName"`
	Options      []string `json:"options"`
	Tradable     *bool    `json:"tradable"`
}

// Item is the game's definition of an item at +0, on every region. EquippedItem is one worn copy with its rolls
type Item struct {
	Region          Region          `json:"region"`
	ID              int             `json:"id"`
	Name            string          `json:"name"`
	Grade           string          `json:"grade"`     // Epic
	GradeName       string          `json:"gradeName"` // localized: Heroic
	CategoryName    string          `json:"categoryName"`
	Type            string          `json:"type"` // Equip, Accessory
	ImageURL        string          `json:"icon"`
	ItemLevel       int             `json:"level"`
	EquipLevel      int             `json:"equipLevel"`
	MaxEnchantLevel int             `json:"maxEnchantLevel"`
	MaxExceedLevel  int             `json:"maxExceedEnchantLevel"`
	RaceName        string          `json:"raceName"`
	ClassNames      []string        `json:"classNames"`
	Tradable        bool            `json:"tradable"`
	MainStats       []ItemStat      `json:"mainStats"`
	SubStats        []ItemStat      `json:"subStats"`     // the roll ranges, MinValue to Value
	SubStatCount    int             `json:"subStatCount"` // how many of SubStats a copy gets
	MagicStoneSlots int             `json:"magicStoneSlotCount"`
	GodStoneSlots   int             `json:"godStoneSlotCount"`
	Costumes        []string        `json:"costumes"`
	Sources         []string        `json:"sources"`
	Raw             json.RawMessage `json:"-"`
}

// ItemGrade is a grade as the dictionary names it. ID is what ItemSearch.Grade takes.
type ItemGrade struct {
	ID   string `json:"id"`
	Name string `json:"name"` // localized; en-US names sit one grade off the ids (Legend reads "Epic")
}

// ItemCategory is a category as the dictionary names it. ID feeds ItemSearch.Category, a child's ID SubCategory.
type ItemCategory struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"` // localized
	Children []ItemCategory `json:"child"`
}

// The values are the official ranking page's own table, read from its bundle (built 2026-04-08)
type RankingContents int

const (
	RankingAbyss              RankingContents = 1
	RankingNightmare          RankingContents = 3
	RankingTranscendence      RankingContents = 4
	RankingArenaOfSolitude    RankingContents = 5
	RankingArenaOfCooperation RankingContents = 6
	RankingAscensionTrial     RankingContents = 21
)

// RankingQuery mirrors the official page's filters. Boards are per server and hold at most 100 rows
type RankingQuery struct {
	ContentsType RankingContents
	ServerID     int
	ClassID      int    // 0 = all classes, for some reason NC calls this rankingType
	Name         string // character name filter
}

type RankingPage struct {
	Region  Region         `json:"region"`
	Season  *Season        `json:"season"`
	Entries []RankingEntry `json:"entries"`
}

type Season struct {
	ContentsType RankingContents `json:"rankingContentsType"`
	State        int             `json:"state"`
	GroupName    string          `json:"groupName"`
	SeasonNo     int             `json:"seasonNo"`
	PrevSeasonNo int             `json:"prevSeasonNo"`
	StartDate    string          `json:"startDate"` // ISO-8601, per NC's schema
	EndDate      string          `json:"endDate"`
}

type RankingEntry struct {
	Ref CharacterRef    `json:"ref"`
	Raw json.RawMessage `json:"-"`

	Rank       int `json:"rank"`
	PrevRank   int `json:"prevRank"`
	RankChange int `json:"rankChange"` // positive means climbed

	IsNew         bool   `json:"isNew"` // if true, the character was not on the board before, RankChange will be 0
	CharacterName string `json:"characterName"`

	ClassID   int    `json:"classId"`
	ClassName string `json:"className"`

	RaceID    int     `json:"raceId"`
	GuildName string  `json:"guildName"`
	Point     float64 `json:"point"`
	GradeName string  `json:"gradeName"`
}

type Flag bool

func (f *Flag) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "1", "true":
		*f = true
	case "0", "false", "null":
		*f = false
	default:
		return fmt.Errorf("aion2: flag is %s, want 0 or 1", data)
	}
	return nil
}

// Lines is a list of strings NC sends as [{"desc": "..."}].
type Lines []string

func (l *Lines) UnmarshalJSON(data []byte) error {
	var rows []descRow
	if err := json.Unmarshal(data, &rows); err != nil {
		var plain []string
		if json.Unmarshal(data, &plain) != nil {
			return err
		}
		*l = plain
		return nil
	}
	*l = nil
	for _, row := range rows {
		*l = append(*l, row.Desc)
	}
	return nil
}

// Board is one of NC's community boards. The first three are NC's own, the rest are the players'
type Board string

const (
	BoardNotices    Board = "notice"   // announcements
	BoardPatchNotes Board = "update"   // one post per weekly maintenance
	BoardDevNews    Board = "cm_story" // the CM team's weekly update news

	BoardFree    Board = "free"           // general discussion
	BoardRecruit Board = "member_recruit" // legion recruitment
	BoardTips    Board = "tip"
	BoardMedia   Board = "image" // screenshots and videos
)

// Post is one board post. HTML is set only by Post, not Posts.
type Post struct {
	ID           string  `json:"id"`
	Board        Board   `json:"board"`
	Region       Region  `json:"region"`
	Title        string  `json:"title"`
	Summary      string  `json:"summary"`
	ThumbnailURL string  `json:"thumbnailUrl"`
	Official     bool    `json:"official"`         // written by NC staff
	Author       *Author `json:"author,omitempty"` // the player who wrote it, nil on staff posts
	Views        int     `json:"views"`
	Comments     int     `json:"comments"`
	HTML         string  `json:"html,omitempty"`

	PostedAt  time.Time `json:"postedAt"` // as NC reports it, it labels local wall-clock time as UTC
	UpdatedAt time.Time `json:"updatedAt"`
}

// Author is the character a player posted as. Ref opens Character, Equipment and the rest.
type Author struct {
	Name string       `json:"name"`
	Ref  CharacterRef `json:"ref"`
}

// Comment is one reply under a post
type Comment struct {
	ID       string    `json:"id"`
	PostID   string    `json:"postId"`
	Text     string    `json:"text"`
	PostedAt time.Time `json:"postedAt"`
	Official bool      `json:"official"`         // written by NC staff
	Author   *Author   `json:"author,omitempty"` // nil on staff comments
}

// StyleTop picks the styleshop's top list: the 50 most downloaded, or liked, looks of a period
type StyleTop struct {
	SortBy string // DOWNLOADS (default) or LIKES
	Period string // DAY_2, DAY_7 (default), DAY_30, DAY_100 or ALL
	Gender string // MALE or FEMALE; empty is both
}

// StyleSearch matches looks by a word in their title and text, or in Field
type StyleSearch struct {
	Keyword string
	Field   string // name (the character's), item (something worn) or tag; empty is title and text
	Page    int    // default 1
	Size    int    // default 20; 100 works
}

// StyleSummary is one styleshop post as the lists show it: a character's look and its screenshots
type StyleSummary struct {
	ID        string    `json:"id"`
	Region    Region    `json:"region"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Author    Author    `json:"author"`
	Images    []string  `json:"images"`
	Views     int       `json:"views"`
	Likes     int       `json:"likes"`
	Downloads int       `json:"downloads"`
	Comments  int       `json:"comments"`
	PostedAt  time.Time `json:"postedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Style is the post itself: the text, the tags, and what the character wore, slot by slot
type Style struct {
	StyleSummary
	Text   string          `json:"text"`
	Tags   []string        `json:"tags"` // localized: gender, class, then the moods the poster picked
	Outfit []StyleSlot     `json:"outfit"`
	Pet    *StyleItem      `json:"pet,omitempty"`
	Wing   *StyleItem      `json:"wing,omitempty"`
	Raw    json.RawMessage `json:"-"`
}

// StyleSlot is one equipment slot of a look: the item worn and the skin over it; either side may be empty
type StyleSlot struct {
	Slot        int    `json:"slot"`
	ItemName    string `json:"itemName"`
	ItemGrade   string `json:"itemGrade"`
	ItemIconURL string `json:"itemIcon"`
	SkinName    string `json:"skinName"`
	SkinGrade   string `json:"skinGrade"`
	SkinIconURL string `json:"skinIcon"`
}

// StyleItem is a look's pet or wing
type StyleItem struct {
	Name    string `json:"itemName"`
	Grade   string `json:"itemGrade"`
	IconURL string `json:"itemIcon"`
}
