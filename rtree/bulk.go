package rtree

import (
	"sort"
)

// BulkItem is an item that can be inserted for bulk loading.
type BulkItem struct {
	Box      Box
	RecordID int
}

// BulkLoad bulk loads multiple items into a new R-Tree. The bulk load
// operation is optimised for creating R-Trees with minimal node overlap. This
// allows for fast searching.
func BulkLoad(items []BulkItem) *RTree {
	if len(items) == 0 {
		return &RTree{}
	}

	levels := calculateLevels(len(items))
	return &RTree{bulkInsert(items, levels)}
}

func calculateLevels(numItems int) int {
	// We could theoretically do this calculation using math.Log. However,
	// float precision issues can cause off-by-one errors in some scenarios.
	// Instead, we calculate the number of levels using integer arithmetic
	// only. This will be fast anyway, since the calculation only requires
	// logarithmic time.
	levels := 1
	count := maxChildren
	for count < numItems {
		count *= maxChildren
		levels++
	}
	return levels
}

func bulkInsert(items []BulkItem, levels int) *node {
	if levels == 1 {
		root := nodePool.Get().(*node)
		root.isLeaf = true
		root.numEntries = len(items)
		for i, item := range items {
			root.entries[i] = entry{
				box:      item.Box,
				recordID: item.RecordID,
			}
		}
		return root
	}

	// NOTE: bulk loading is hardcoded around the fact that the min and max
	// node cardinalities are 2 and 4.

	// 6 is the first number of items that can be split into 3 nodes while
	// respecting the minimum node cardinality, i.e. 6 = 2 + 2 + 2. Anything
	// less than 6 must instead be split into 2 nodes.
	if len(items) < 6 {
		firstHalf, secondHalf := splitBulkItems2Ways(items)
		return bulkNode(levels, firstHalf, secondHalf)
	}

	// 8 is the first number of items that can be split into 4 nodes while
	// respecting the minimum node cardinality, i.e. 8 = 2 + 2 + 2 + 2.
	// Anything less that 8 must instead be split into 3 nodes.
	if len(items) < 8 {
		firstThird, secondThird, thirdThird := splitBulkItems3Ways(items)
		return bulkNode(levels, firstThird, secondThird, thirdThird)
	}

	// 4-way split:
	firstHalf, secondHalf := splitBulkItems2Ways(items)
	firstQuarter, secondQuarter := splitBulkItems2Ways(firstHalf)
	thirdQuarter, fourthQuarter := splitBulkItems2Ways(secondHalf)
	return bulkNode(levels, firstQuarter, secondQuarter, thirdQuarter, fourthQuarter)
}

func bulkNode(levels int, parts ...[]BulkItem) *node {
	root := nodePool.Get().(*node)
	root.numEntries = len(parts)
	for i, part := range parts {
		child := bulkInsert(part, levels-1)
		child.parent = root
		root.entries[i].child = child
		root.entries[i].box = calculateBound(child)
	}
	return root
}

func splitBulkItems2Ways(items []BulkItem) ([]BulkItem, []BulkItem) {
	sortBulkItems(items)
	split := len(items) / 2
	return items[:split], items[split:]
}

func splitBulkItems3Ways(items []BulkItem) ([]BulkItem, []BulkItem, []BulkItem) {
	sortBulkItems(items)
	cutA := len(items) / 3
	cutB := cutA
	remaining := len(items)/3 - 3*cutA
	if remaining == 1 {
		cutA++
	}
	if remaining == 2 {
		cutA++
		cutB++
	}
	return items[:cutA], items[cutA : cutA+cutB], items[cutA+cutB:]
}

func sortBulkItems(items []BulkItem) {
	box := items[0].Box
	for _, item := range items[1:] {
		box = combine(box, item.Box)
	}
	bulkItems := bulkItems{
		horizontal: box.MaxX-box.MinX > box.MaxY-box.MinY,
		items:      items,
	}
	sort.Sort(bulkItems)
}

// bulkItems implements the sort.Interface interface. This style of sorting is
// used rather than sort.Slice because it does less allocations.
type bulkItems struct {
	horizontal bool
	items      []BulkItem
}

func (b bulkItems) Len() int {
	return len(b.items)
}
func (b bulkItems) Less(i, j int) bool {
	bi := b.items[i].Box
	bj := b.items[j].Box
	if b.horizontal {
		return bi.MinX+bi.MaxX < bj.MinX+bj.MaxX
	} else {
		return bi.MinY+bi.MaxY < bj.MinY+bj.MaxY
	}
}
func (b bulkItems) Swap(i, j int) {
	b.items[i], b.items[j] = b.items[j], b.items[i]
}
