package mouse

type Pointer interface {
	MoveTo(x, y int32) error
	ClickAt(x, y int32) error
	RightClickAt(x, y int32) error
	Close() error
}
