package aion2

import (
	"net/url"
	"strconv"
	"strings"
)

func (q CharacterSearch) query() url.Values {
	// @REVIEW: rewrite this shit
	v := url.Values{
		"keyword": {q.Keyword},
		"race":    {strconv.Itoa(q.RaceID)}, // 1 = Ely, 2 = Asmo, empty race will be HTTP 400
		"page":    {strconv.Itoa(max(q.Page, 1))},
		"size":    {strconv.Itoa(q.Size)},
	}
	if q.Size <= 0 {
		v.Set("size", "40")
	}
	if q.ServerID > 0 {
		v.Set("serverId", strconv.Itoa(q.ServerID))
	}
	if q.Sort != "" {
		v.Set("sort", q.Sort)
	}
	return v
}

func (q ItemSearch) query() url.Values {
	// @TODO: find a better way to get all query params
	v := url.Values{
		"page": {strconv.Itoa(max(q.Page, 1))},
		"size": {strconv.Itoa(q.Size)},
	}
	if q.Size <= 0 {
		v.Set("size", "30")
	}
	for param, value := range map[string]string{
		"searchKeyword": q.Query,
		"grades":        q.Grade,
		"category1":     q.Category,
		"category2":     q.SubCategory,
	} {
		if value != "" {
			v.Set(param, value)
		}
	}
	return v
}

func (q RankingQuery) query(lang Locale) url.Values {
	v := url.Values{
		"lang":                {string(lang)},
		"rankingContentsType": {strconv.Itoa(int(q.ContentsType))},
		"rankingType":         {strconv.Itoa(q.ClassID)},
		"serverId":            {strconv.Itoa(q.ServerID)},
	}
	if q.Name != "" {
		v.Set("searchCharacterName", q.Name)
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
