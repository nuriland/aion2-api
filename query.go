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
