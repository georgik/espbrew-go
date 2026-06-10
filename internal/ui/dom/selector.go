//go:build js
// +build js

package dom

// Selector provides helper methods for element selection
type Selector struct {
	doc *Document
}

// NewSelector creates a new selector helper
func NewSelector(doc *Document) *Selector {
	return &Selector{doc: doc}
}

// ID returns an element by its ID
func (s *Selector) ID(id string) *Element {
	return s.doc.GetElementByID(id)
}

// Class returns the first element with the given class
func (s *Selector) Class(class string) *Element {
	return s.doc.QuerySelector("." + class)
}

// ClassAll returns all elements with the given class
func (s *Selector) ClassAll(class string) []*Element {
	return s.doc.QuerySelectorAll("." + class)
}

// Tag returns the first element with the given tag name
func (s *Selector) Tag(tag string) *Element {
	return s.doc.QuerySelector(tag)
}

// TagAll returns all elements with the given tag name
func (s *Selector) TagAll(tag string) []*Element {
	return s.doc.QuerySelectorAll(tag)
}

// Name returns the first element with the given name attribute
func (s *Selector) Name(name string) *Element {
	return s.doc.QuerySelector("[name=\"" + name + "\"]")
}

// NameAll returns all elements with the given name attribute
func (s *Selector) NameAll(name string) []*Element {
	return s.doc.QuerySelectorAll("[name=\"" + name + "\"]")
}

// Data returns the first element with the given data attribute
func (s *Selector) Data(key string) *Element {
	return s.doc.QuerySelector("[data-" + key + "]")
}

// DataValue returns the first element with the given data attribute value
func (s *Selector) DataValue(key, value string) *Element {
	return s.doc.QuerySelector("[data-" + key + "=\"" + value + "\"]")
}

// DataAll returns all elements with the given data attribute
func (s *Selector) DataAll(key string) []*Element {
	return s.doc.QuerySelectorAll("[data-" + key + "]")
}

// SelectorQuery returns elements matching any CSS selector
func (s *Selector) Query(selector string) *Element {
	return s.doc.QuerySelector(selector)
}

// SelectorQueryAll returns all elements matching any CSS selector
func (s *Selector) QueryAll(selector string) []*Element {
	return s.doc.QuerySelectorAll(selector)
}

// Ancestor returns the first ancestor matching the selector
func (s *Selector) Ancestor(elem *Element, selector string) *Element {
	if elem == nil {
		return nil
	}

	parent := elem.GetParent()
	for parent != nil {
		if parent.Matches(selector) {
			return parent
		}
		parent = parent.GetParent()
	}
	return nil
}

// Ancestors returns all ancestors matching the selector
func (s *Selector) Ancestors(elem *Element, selector string) []*Element {
	if elem == nil {
		return nil
	}

	result := []*Element{}
	parent := elem.GetParent()

	for parent != nil {
		if parent.Matches(selector) {
			result = append(result, parent)
		}
		parent = parent.GetParent()
	}

	return result
}

// === Element helper methods that use selectors ===

// Matches returns true if the element matches the selector
func (e *Element) Matches(selector string) bool {
	if e == nil {
		return false
	}

	// Use matches method if available
	if fn := e.value.Get("matches"); !fn.IsUndefined() {
		return e.value.Call("matches", selector).Bool()
	}

	// Fallback: use querySelector on parent and compare
	parent := e.GetParent()
	if parent == nil {
		return false
	}

	matched := parent.QuerySelector(selector)
	return matched != nil && matched.value.Equal(e.value)
}

// Closest returns the first ancestor that matches the selector (including the element itself)
func (e *Element) Closest(selector string) *Element {
	if e == nil {
		return nil
	}

	// Check if element itself matches
	if e.Matches(selector) {
		return e
	}

	// Check ancestors
	doc := GlobalDocument()
	if doc == nil {
		return nil
	}
	sel := NewSelector(doc)
	return sel.Ancestor(e, selector)
}

// Find returns the first descendant element matching the selector
func (e *Element) Find(selector string) *Element {
	return e.QuerySelector(selector)
}

// FindAll returns all descendant elements matching the selector
func (e *Element) FindAll(selector string) []*Element {
	return e.QuerySelectorAll(selector)
}

// Children returns the direct children that match the selector
func (e *Element) ChildrenMatching(selector string) []*Element {
	if e == nil {
		return nil
	}

	children := e.GetChildren()
	result := []*Element{}

	for _, child := range children {
		if child.Matches(selector) {
			result = append(result, child)
		}
	}

	return result
}

// Siblings returns all sibling elements that match the selector
func (e *Element) Siblings(selector string) []*Element {
	if e == nil {
		return nil
	}

	parent := e.GetParent()
	if parent == nil {
		return nil
	}

	siblings := parent.GetChildren()
	result := []*Element{}

	for _, sibling := range siblings {
		if sibling != e && sibling.Matches(selector) {
			result = append(result, sibling)
		}
	}

	return result
}

// Next returns the next sibling element that matches the selector
func (e *Element) NextMatching(selector string) *Element {
	if e == nil {
		return nil
	}

	next := e.GetNextSibling()
	for next != nil {
		if next.Matches(selector) {
			return next
		}
		next = next.GetNextSibling()
	}

	return nil
}

// Previous returns the previous sibling element that matches the selector
func (e *Element) PreviousMatching(selector string) *Element {
	if e == nil {
		return nil
	}

	prev := e.GetPreviousSibling()
	for prev != nil {
		if prev.Matches(selector) {
			return prev
		}
		prev = prev.GetPreviousSibling()
	}

	return nil
}

// Parent returns the closest ancestor that matches the selector
func (e *Element) Parent(selector string) *Element {
	if e == nil {
		return nil
	}

	parent := e.GetParent()
	for parent != nil {
		if parent.Matches(selector) {
			return parent
		}
		parent = parent.GetParent()
	}

	return nil
}

// Parents returns all ancestors that match the selector
func (e *Element) Parents(selector string) []*Element {
	if e == nil {
		return nil
	}

	result := []*Element{}
	parent := e.GetParent()

	for parent != nil {
		if parent.Matches(selector) {
			result = append(result, parent)
		}
		parent = parent.GetParent()
	}

	return result
}

// Until returns all ancestors up to (but not including) the element matching the until selector
func (e *Element) Until(selector string) []*Element {
	if e == nil {
		return nil
	}

	result := []*Element{}
	parent := e.GetParent()

	for parent != nil {
		if parent.Matches(selector) {
			break
		}
		result = append(result, parent)
		parent = parent.GetParent()
	}

	return result
}

// Filter returns elements from the list that match the selector
func Filter(elems []*Element, selector string) []*Element {
	result := []*Element{}

	for _, elem := range elems {
		if elem.Matches(selector) {
			result = append(result, elem)
		}
	}

	return result
}

// Not returns elements from the list that do not match the selector
func Not(elems []*Element, selector string) []*Element {
	result := []*Element{}

	for _, elem := range elems {
		if !elem.Matches(selector) {
			result = append(result, elem)
		}
	}

	return result
}

// First returns the first element from the list
func First(elems []*Element) *Element {
	if len(elems) == 0 {
		return nil
	}
	return elems[0]
}

// Last returns the last element from the list
func Last(elems []*Element) *Element {
	if len(elems) == 0 {
		return nil
	}
	return elems[len(elems)-1]
}

// Eq returns the element at the specified index
func Eq(elems []*Element, index int) *Element {
	if index < 0 || index >= len(elems) {
		return nil
	}
	return elems[index]
}

// Slice returns a slice of elements
func Slice(elems []*Element, start, end int) []*Element {
	if start < 0 {
		start = 0
	}
	if end > len(elems) {
		end = len(elems)
	}
	if start >= end {
		return []*Element{}
	}
	return elems[start:end]
}

// Each calls the function for each element
func Each(elems []*Element, fn func(int, *Element)) {
	for i, elem := range elems {
		fn(i, elem)
	}
}

// Map transforms elements using the function
func Map(elems []*Element, fn func(int, *Element) interface{}) []interface{} {
	result := make([]interface{}, len(elems))

	for i, elem := range elems {
		result[i] = fn(i, elem)
	}

	return result
}

// Is returns true if the element is in the list
func Is(elems []*Element, elem *Element) bool {
	for _, e := range elems {
		if e.value.Equal(elem.value) {
			return true
		}
	}
	return false
}

// Index returns the index of the element in the list, or -1 if not found
func Index(elems []*Element, elem *Element) int {
	for i, e := range elems {
		if e.value.Equal(elem.value) {
			return i
		}
	}
	return -1
}

// Add adds elements to the list
func Add(elems []*Element, newElems ...*Element) []*Element {
	result := make([]*Element, len(elems), len(elems)+len(newElems))
	copy(result, elems)
	return append(result, newElems...)
}

// NotIndex removes the element at the specified index
func NotIndex(elems []*Element, index int) []*Element {
	if index < 0 || index >= len(elems) {
		return elems
	}
	result := make([]*Element, len(elems)-1)
	copy(result, elems[:index])
	copy(result[index:], elems[index+1:])
	return result
}
