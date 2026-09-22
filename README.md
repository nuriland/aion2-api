# aion2

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

item, _ := tw.Item(ctx, 110120001) // first call crawls the catalog, ~1 min. after that it's instant
```
