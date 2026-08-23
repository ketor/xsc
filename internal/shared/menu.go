package shared

// MenuItem 定义菜单中的可执行项。
type MenuItem struct {
	Label    string
	Shortcut string
	Action   string
	Disabled bool
}

// Menu 定义顶部菜单及其下拉项。
type Menu struct {
	Label string
	Items []MenuItem
}

// MenuState 保存顶部菜单的交互状态。
type MenuState struct {
	Open   bool
	Active int
	Cursor int
}

func (s *MenuState) OpenMenu(index int, menus []Menu) {
	if index < 0 || index >= len(menus) {
		s.Close()
		return
	}
	s.Open = true
	s.Active = index
	s.Cursor = firstEnabledMenuItem(menus[index].Items)
}

func (s *MenuState) Close() {
	s.Open = false
	s.Active = 0
	s.Cursor = 0
}

func (s *MenuState) MoveMenu(delta int, menus []Menu) {
	if len(menus) == 0 {
		return
	}
	next := (s.Active + delta) % len(menus)
	if next < 0 {
		next += len(menus)
	}
	s.OpenMenu(next, menus)
}

func (s *MenuState) MoveItem(delta int, menus []Menu) {
	if !s.Open || s.Active < 0 || s.Active >= len(menus) {
		return
	}
	items := menus[s.Active].Items
	if len(items) == 0 {
		return
	}
	for range len(items) {
		s.Cursor = (s.Cursor + delta) % len(items)
		if s.Cursor < 0 {
			s.Cursor += len(items)
		}
		if !items[s.Cursor].Disabled {
			return
		}
	}
}

func (s MenuState) Selected(menus []Menu) (MenuItem, bool) {
	if !s.Open || s.Active < 0 || s.Active >= len(menus) {
		return MenuItem{}, false
	}
	items := menus[s.Active].Items
	if s.Cursor < 0 || s.Cursor >= len(items) || items[s.Cursor].Disabled {
		return MenuItem{}, false
	}
	return items[s.Cursor], true
}

// MenuIndexAtX 返回顶部菜单栏中横坐标对应的菜单。
func MenuIndexAtX(menus []Menu, x int) int {
	start := 0
	for index, menu := range menus {
		end := start + menuLabelWidth(menu.Label)
		if x >= start && x < end {
			return index
		}
		start = end
	}
	return -1
}

func MenuStartX(menus []Menu, index int) int {
	x := 0
	for i := range index {
		x += menuLabelWidth(menus[i].Label)
	}
	return x
}

func menuLabelWidth(label string) int {
	return len([]rune(label)) + 2
}

func firstEnabledMenuItem(items []MenuItem) int {
	for index, item := range items {
		if !item.Disabled {
			return index
		}
	}
	return 0
}
