package aion2

import (
	"net/url"
	"strconv"
	"strings"
)

// query leaves serverId out when unset: NC treats serverId=0 as a server that
// does not exist and answers with zero matches.
func (q CharacterSearch) query() url.Values {
	size := q.Size
	if size <= 0 {
		size = 40
	}
	v := url.Values{
		"keyword": {q.Keyword},
		"race":    {strconv.Itoa(q.RaceID)}, // 1 = Ely, 2 = Asmo
		"page":    {strconv.Itoa(max(q.Page, 1))},
		"size":    {strconv.Itoa(size)},
	}
	if q.ServerID > 0 {
		v.Set("serverId", strconv.Itoa(q.ServerID))
	}
	if q.Sort != "" {
		v.Set("sort", q.Sort)
	}
	return v
}

// query uses the dictionary's own param names. The obvious ones (query, grade, category) are accepted
// and silently ignored, which returns the whole catalog and looks like success.
func (q ItemSearch) query() url.Values {
	size := q.Size
	if size <= 0 {
		size = 30
	}
	v := url.Values{
		"page": {strconv.Itoa(max(q.Page, 1))},
		"size": {strconv.Itoa(size)},
	}
	if q.Query != "" {
		v.Set("searchKeyword", q.Query)
	}
	if q.Grade != "" {
		v.Set("grades", q.Grade)
	}
	if q.Category != "" {
		v.Set("category1", q.Category)
	}
	if q.SubCategory != "" {
		v.Set("category2", q.SubCategory)
	}
	return v
}

// query is the official ranking page's own filters. rankingType is the class, 0 for all.
func (q RankingQuery) query() url.Values {
	v := url.Values{
		"rankingContentsType": {strconv.Itoa(int(q.ContentsType))},
		"rankingType":         {strconv.Itoa(q.ClassID)},
		"serverId":            {strconv.Itoa(q.ServerID)},
	}
	if q.Name != "" {
		v.Set("searchCharacterName", q.Name)
	}
	return v
}

// query asks for 100 rows, the list is 50 today, so one page holds a cap NC doubles
func (q StyleTop) query() url.Values {
	v := url.Values{"size": {"100"}}
	if q.SortBy != "" {
		v.Set("sortBy", q.SortBy)
	}
	if q.Period != "" {
		v.Set("period", q.Period)
	}
	if q.Gender != "" {
		v.Set("charGender", q.Gender)
	}
	return v
}

// query counts pages from 0 where the SDK counts from 1. Page and Size are already set
func (q StyleSearch) query() url.Values {
	v := url.Values{
		"page": {strconv.Itoa(q.Page - 1)},
		"size": {strconv.Itoa(q.Size)},
	}
	if q.Keyword != "" {
		v.Set("keyword", q.Keyword)
	}
	if q.Field != "" {
		v.Set("field", q.Field)
	}
	if q.Gender != "" {
		v.Set("charGender", q.Gender)
	}
	return v
}

// dictLocale is the full tag the dictionary wants where the API takes a short one
func dictLocale(l Locale) string {
	switch l {
	case LocaleKO:
		return "ko-KR"
	case LocaleEN:
		return "en-US"
	}
	return string(l)
}

// decodeCharacterID undoes the percent-encoding that search results and official profile URLs carry (…aCA%3D).
// url.Values would otherwise encode the id a second time, and NC answers a double-encoded id with 404.
//
// The decoded alphabet is base64url plus '=', so a '%' can only be an escape.
func decodeCharacterID(id string) string {
	if !strings.Contains(id, "%") {
		return id
	}
	decoded, err := url.PathUnescape(id)
	if err != nil {
		return id
	}
	return decoded
}
