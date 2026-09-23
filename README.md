# aion2

Unofficial client for the AION 2 website's JSON API. Supports KR and TW. Global coming as soon as the API is available.

```go
import aion2 "github.com/nuriland/aion2-api"

kr, _ := aion2.New(aion2.ConfigOpts{Region: aion2.RegionKR, Locale: aion2.LocaleEN})
tw, _ := aion2.New(aion2.ConfigOpts{Region: aion2.RegionTW})
```

## Servers and classes

```go
servers, _ := kr.Servers(ctx) // 42 on KR, 36 on TW
classes, _ := kr.Classes(ctx)
```

## Characters

```go
res, _ := tw.SearchCharacters(ctx, aion2.CharacterSearch{
    Keyword:  "a",
    RaceID:   1,        // 1 Ely, 2 Asmo
    ServerID: 1001,     // optional
    ClassIDs: []int{2}, // optional
})

ref := res.Items[0].Ref

ch, _    := tw.Character(ctx, ref)
eq, _    := tw.Equipment(ctx, ref)
item, _  := tw.EquippedItem(ctx, ref, eq.Slots[0])
board, _ := tw.Daevanion(ctx, ref, ch.Daevanion[0].ID)

fmt.Println(ch.Profile.Name, ch.Profile.CombatPower, len(eq.Slots))

for hit, err := range tw.Characters(ctx, aion2.CharacterSearch{Keyword: "a", RaceID: 1}) {
    if err != nil {
        return err
    }
    fmt.Println(hit.Name, hit.Level)
}
```

Got a profile URL instead? Paste the id straight in:

```go
ch, _ := kr.Character(ctx, aion2.CharacterRef{
    ServerID:    1001,
    CharacterID: "<the id from the profile URL>",
})
```

## Items

TW only.

```go
page, _ := tw.SearchItems(ctx, aion2.ItemSearch{
    Query:       "巨劍",
    Grade:       "Epic",
    Category:    "Equip_Weapon",
    SubCategory: "Greatsword",
    ClassID:     2,
    Size:        200,
})

for it, err := range tw.Items(ctx, aion2.ItemSearch{Grade: "Epic", Category: "Equip_Weapon"}) {
    if err != nil {
        return err
    }
    fmt.Println(it.ID, it.Name)
}

item, _ := tw.Item(ctx, 110120001) // first call crawls the catalog once

grades, _ := tw.ItemGrades(ctx)         // the ids ItemSearch.Grade takes, with localized names
categories, _ := tw.ItemCategories(ctx) // same for Category / SubCategory
```

## Rankings

NC took the public boards down in 2026 and until they return, every board answers `ErrNoSeason`.

```go
board, err := kr.Rankings(ctx, aion2.RankingQuery{ContentsType: aion2.RankingAbyss, ServerID: 1001})
if errors.Is(err, aion2.ErrNoSeason) {
    return nil
}
fmt.Println(board.Season.SeasonNo, len(board.Entries))
```

## News

```go
notes, _ := kr.Posts(ctx, aion2.BoardPatchNotes) // also BoardDevNews (the CM team's weekly news), BoardNotices
latest, _ := kr.Post(ctx, aion2.BoardPatchNotes, notes[0].ID) // .HTML is the body
pinned, _ := kr.PinnedPosts(ctx, aion2.BoardNotices)

fmt.Println(latest.Title, latest.PostedAt, len(latest.HTML))
```

### Player boards 

```go
posts, _ := tw.Posts(ctx, aion2.BoardFree) // also BoardRecruit, BoardTips, BoardMedia
ch, _ := tw.Character(ctx, posts[0].Author.Ref)
comments, _ := tw.Comments(ctx, aion2.BoardFree, posts[0].ID)
```

