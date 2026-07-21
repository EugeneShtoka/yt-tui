package videotable

import etable "github.com/evertras/bubble-table/table"

// HasActivityDetail is implemented by activity-log row types.
type HasActivityDetail interface {
	GetActivityDetail() string
}

// ActDetailCol renders the locality-aware activity description as a flex column.
func ActDetailCol[T HasActivityDetail]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewFlexColumn(KeyActDetail, "Detail", 1),
		Cell: func(item T, _ int) any { return item.GetActivityDetail() },
	}
}
