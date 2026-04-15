package provider

// FieldHint tells the DCL converter how to represent a nested block group.
type FieldHint int

const (
	// FieldBlockList means the block should always produce a ListVal,
	// even when only one block of that type appears.
	FieldBlockList FieldHint = iota + 1

	// FieldBlockMap means the block should always produce a MapVal,
	// even when multiple blocks of that type appear (which would be an error).
	FieldBlockMap
)

// Schema declares the expected structure for a resource type's nested blocks.
// The converter uses these hints to choose ListVal vs MapVal instead of
// guessing from occurrence count.
type Schema struct {
	Fields map[string]FieldHint
}
