package parser

var exists = struct{}{}

type set struct {
	m map[string]struct{}
}

// NewSet constructs a new set based on map implementation
func NewSet(elements []string) *set {
	s := set{}
	s.m = make(map[string]struct{})
	for _, e := range elements {
		s.Add(e)
	}
	return &s
}

func (s *set) Add(e string) {
	s.m[e] = exists
}

func (s *set) Remove(e string) {
	delete(s.m, e)
}

func (s *set) Contain(e string) bool {
	_, ok := s.m[e]
	return ok
}

func (s *set) ToSlice() []string {
	keys := make([]string, len(s.m))

	i := 0
	for key := range s.m {
		keys[i] = key
		i++
	}

	return keys
}
