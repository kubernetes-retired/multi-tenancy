//go:generate go run generator.go

package box

type embedBox struct {
	storage map[string][]byte
}

// Create new box for embed files
func newEmbedBox() *embedBox {
	return &embedBox{storage: make(map[string][]byte)}
}

// Add a file to box
func (e *embedBox) Add(file string, content []byte) {
	e.storage[file] = content
}

// Get file's content
func (e *embedBox) Get(file string) []byte {
	if f, ok := e.storage[file]; ok {
		return f
	}
	return nil
}

// Embed box expose
var box = newEmbedBox()

// Add a file content to box
func Add(file string, content []byte) {
	box.Add(file, content)
}

// Get a file from box
func Get(file string) []byte {
	return box.Get(file)
}
