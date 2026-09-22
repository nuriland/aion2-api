package aion2

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	classTableCacheTTL = 24 * time.Hour // How stale the class table and the item index may get
	itemIndexCacheTTL  = 24 * time.Hour // How stale the item index may get
)

// classTable joins the two ideas of a class. Class.ID (2 = Gladiator) is what callers hold.
//
// A pcId is one class x race x gender combination. 36 in all, and it is the only form character search sends or accepts.
type classTable struct {
	byID   map[int]Class
	byPcID map[int]Class
	pcIDs  map[int][]int // Class.ID -> its pcIds
}

// newClassTable joins the two lists on the class slug, which pcdata upper-cases: GLADIATOR there, Gladiator in the class list.
//
// A pcdata row naming a class the class list lacks is skipped.
func newClassTable(classes []Class, pcs []pcRow) *classTable {
	var (
		byID   = make(map[int]Class, len(classes))
		byPcID = make(map[int]Class, len(pcs))
		pcIDs  = make(map[int][]int, len(classes))
		bySlug = make(map[string]Class, len(classes))

		ct = classTable{
			byID:   byID,
			byPcID: byPcID,
			pcIDs:  pcIDs,
		}
	)
	for _, class := range classes {
		byID[class.ID] = class
		bySlug[strings.ToUpper(class.Name)] = class
	}
	for _, pc := range pcs {
		if class, ok := bySlug[pc.ClassName]; ok {
			byPcID[pc.ID] = class
			pcIDs[class.ID] = append(pcIDs[class.ID], pc.ID)
		}
	}
	return &ct
}

func (t *classTable) knows(pcIDs []int) bool {
	for _, id := range pcIDs {
		if _, ok := t.byPcID[id]; !ok {
			return false
		}
	}
	return true
}

// pcIDParam is the pcId query value selecting every combination of the given classes: "5,6,7,8" for Gladiator.
func (t *classTable) pcIDParam(classIDs []int) (string, error) {
	var ids []string
	for _, classID := range classIDs {
		pcIDs, ok := t.pcIDs[classID]
		if !ok {
			return "", fmt.Errorf("unknown class id %d", classID)
		}
		for _, id := range pcIDs {
			ids = append(ids, strconv.Itoa(id))
		}
	}
	return strings.Join(ids, ","), nil
}
