package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/DiLRandI/just-a-todo/internal/dateparse"
	"github.com/DiLRandI/just-a-todo/internal/store"
	"github.com/DiLRandI/just-a-todo/internal/todo"
)

type screenMode int

const (
	screenList screenMode = iota
	screenForm
	screenSearch
	screenConfirmDelete
)

type viewMode string

const (
	viewOpen      viewMode = "open"
	viewToday     viewMode = "today"
	viewTomorrow  viewMode = "tomorrow"
	viewThisWeek  viewMode = "this week"
	viewNextWeek  viewMode = "next week"
	viewThisMonth viewMode = "this month"
	viewNextMonth viewMode = "next month"
	viewNoDue     viewMode = "no due"
	viewDone      viewMode = "done"
	viewArchived  viewMode = "archived"
)

type Model struct {
	ctx       context.Context
	store     *store.Store
	items     []todo.Todo
	cursor    int
	offset    int
	width     int
	height    int
	screen    screenMode
	view      viewMode
	search    string
	message   string
	err       error
	inputs    []textinput.Model
	notes     textarea.Model
	field     int
	pick      int
	editingID int64
	confirmID int64
}

const (
	formTitle = iota
	formDue
	formPriority
	formTags
	formRepeat
	formNotes
)

type suggestion struct {
	value string
	label string
}

func Run(ctx context.Context, st *store.Store) error {
	model := NewModel(ctx, st)
	program := tea.NewProgram(model)
	_, err := program.Run()
	return err
}

func NewModel(ctx context.Context, st *store.Store) Model {
	model := Model{
		ctx:    ctx,
		store:  st,
		view:   viewOpen,
		width:  100,
		height: 30,
	}
	model.refresh()
	return model
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampCursor()
		return m, nil
	case tea.KeyPressMsg:
		switch m.screen {
		case screenForm:
			return m.updateForm(msg)
		case screenSearch:
			return m.updateSearch(msg)
		case screenConfirmDelete:
			return m.updateConfirmDelete(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	var content string
	switch m.screen {
	case screenForm:
		content = m.formView()
	case screenSearch:
		content = m.searchView()
	case screenConfirmDelete:
		content = m.confirmDeleteView()
	default:
		content = m.listView()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "todo"
	return view
}

func (m Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.message = ""
	m.err = nil

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = len(m.items) - 1
	case "r":
		m.refresh()
	case "1":
		m.setView(viewOpen)
	case "2":
		m.setView(viewToday)
	case "3":
		m.setView(viewTomorrow)
	case "4":
		m.setView(viewThisWeek)
	case "5":
		m.setView(viewNextWeek)
	case "6":
		m.setView(viewThisMonth)
	case "7":
		m.setView(viewNextMonth)
	case "8":
		m.setView(viewNoDue)
	case "9":
		m.setView(viewDone)
	case "0":
		m.setView(viewArchived)
	case "n":
		m.startCreate()
		return m, m.focusField()
	case "e":
		if item, ok := m.selected(); ok {
			m.startEdit(item)
			return m, m.focusField()
		}
	case "/":
		m.startSearch()
		return m, m.focusField()
	case "esc":
		m.search = ""
		m.setView(viewOpen)
	case "enter", "space", "d":
		m.toggleDone()
	case "a":
		m.archiveSelected()
	case "u":
		m.unarchiveSelected()
	case "x":
		if item, ok := m.selected(); ok {
			m.screen = screenConfirmDelete
			m.confirmID = item.ID
		}
	}

	m.clampCursor()
	return m, nil
}

func (m Model) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.message = ""
	switch msg.String() {
	case "ctrl+c", "esc":
		m.screen = screenList
		m.inputs = nil
		m.editingID = 0
		return m, nil
	case "up":
		if m.hasSuggestions() {
			m.movePick(-1)
			return m, nil
		}
	case "down":
		if m.hasSuggestions() {
			m.movePick(1)
			return m, nil
		}
	case "shift+tab":
		m.moveField(-1)
		return m, m.focusField()
	case "tab":
		m.moveField(1)
		return m, m.focusField()
	case "enter":
		if m.hasSuggestions() {
			m.applyPick()
			m.moveField(1)
			return m, m.focusField()
		}
		if m.field == formNotes {
			updated, cmd := m.notes.Update(msg)
			m.notes = updated
			return m, cmd
		}
		if m.field < formNotes {
			m.moveField(1)
			return m, m.focusField()
		}
		return m.saveForm(), nil
	case "ctrl+s":
		return m.saveForm(), nil
	}

	var cmd tea.Cmd
	if m.field == formNotes {
		m.notes, cmd = m.notes.Update(msg)
	} else {
		var updated textinput.Model
		updated, cmd = m.inputs[m.field].Update(msg)
		m.inputs[m.field] = updated
	}
	if m.hasSuggestions() {
		m.clampPick()
	}
	return m, cmd
}

func (m Model) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.screen = screenList
		return m, nil
	case "enter":
		m.search = strings.TrimSpace(m.inputs[0].Value())
		m.screen = screenList
		m.refresh()
		return m, nil
	}
	updated, cmd := m.inputs[0].Update(msg)
	m.inputs[0] = updated
	return m, cmd
}

func (m Model) updateConfirmDelete(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc", "n":
		m.screen = screenList
		m.confirmID = 0
	case "y":
		if err := m.store.DeleteTodo(m.ctx, m.confirmID); err != nil {
			m.err = err
		} else {
			m.message = fmt.Sprintf("Removed #%d", m.confirmID)
		}
		m.screen = screenList
		m.confirmID = 0
		m.refresh()
	}
	return m, nil
}

func (m *Model) refresh() {
	filter := store.ListFilter{Search: m.search}
	now := time.Now()
	switch m.view {
	case viewToday:
		m.applyDateRange(&filter, "today", now)
	case viewTomorrow:
		m.applyDateRange(&filter, "tomorrow", now)
	case viewThisWeek:
		m.applyDateRange(&filter, "this week", now)
	case viewNextWeek:
		m.applyDateRange(&filter, "next week", now)
	case viewThisMonth:
		m.applyDateRange(&filter, "this month", now)
	case viewNextMonth:
		m.applyDateRange(&filter, "next month", now)
	case viewNoDue:
		filter.NoDueOnly = true
	case viewDone:
		filter.Status = todo.StatusDone
	case viewArchived:
		filter.ArchivedOnly = true
		filter.IncludeArchived = true
		filter.AllStatuses = true
	default:
		m.view = viewOpen
	}

	items, err := m.store.ListTodos(m.ctx, filter)
	if err != nil {
		m.err = err
		return
	}
	m.items = items
	m.clampCursor()
}

func (m *Model) applyDateRange(filter *store.ListFilter, name string, now time.Time) {
	r, err := dateparse.RangeFor(name, now)
	if err != nil {
		m.err = err
		return
	}
	filter.DueStart = r.StartDate
	filter.DueEnd = r.EndDate
}

func (m *Model) setView(view viewMode) {
	m.view = view
	m.cursor = 0
	m.offset = 0
	m.refresh()
}

func (m *Model) clampCursor() {
	if len(m.items) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	visible := max(5, m.height-10)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) selected() (todo.Todo, bool) {
	if len(m.items) == 0 || m.cursor < 0 || m.cursor >= len(m.items) {
		return todo.Todo{}, false
	}
	return m.items[m.cursor], true
}

func (m *Model) toggleDone() {
	item, ok := m.selected()
	if !ok {
		return
	}
	if item.Status == todo.StatusDone {
		if err := m.store.ReopenTodo(m.ctx, item.ID); err != nil {
			m.err = err
		} else {
			m.message = fmt.Sprintf("Reopened #%d", item.ID)
		}
	} else {
		done, next, err := m.store.CompleteTodo(m.ctx, item.ID, time.Now())
		if err != nil {
			m.err = err
		} else if next != nil {
			m.message = fmt.Sprintf("Done #%d; next is #%d due %s", done.ID, next.ID, next.DueLabel())
		} else {
			m.message = fmt.Sprintf("Done #%d", done.ID)
		}
	}
	m.refresh()
}

func (m *Model) archiveSelected() {
	item, ok := m.selected()
	if !ok {
		return
	}
	if err := m.store.ArchiveTodo(m.ctx, item.ID); err != nil {
		m.err = err
	} else {
		m.message = fmt.Sprintf("Archived #%d", item.ID)
	}
	m.refresh()
}

func (m *Model) unarchiveSelected() {
	item, ok := m.selected()
	if !ok {
		return
	}
	if err := m.store.UnarchiveTodo(m.ctx, item.ID); err != nil {
		m.err = err
	} else {
		m.message = fmt.Sprintf("Unarchived #%d", item.ID)
	}
	m.refresh()
}

func (m *Model) startCreate() {
	m.screen = screenForm
	m.editingID = 0
	m.field = 0
	m.pick = 0
	m.inputs, m.notes = newFormInputs(todo.Todo{})
}

func (m *Model) startEdit(item todo.Todo) {
	m.screen = screenForm
	m.editingID = item.ID
	m.field = 0
	m.pick = 0
	m.inputs, m.notes = newFormInputs(item)
}

func (m *Model) startSearch() {
	m.screen = screenSearch
	m.field = 0
	input := textinput.New()
	input.Placeholder = "search"
	input.Prompt = "Search: "
	input.SetWidth(max(20, m.width-12))
	input.SetValue(m.search)
	m.inputs = []textinput.Model{input}
}

func newFormInputs(item todo.Todo) ([]textinput.Model, textarea.Model) {
	labels := []string{"Title", "Due", "Priority", "Tags", "Repeat"}
	placeholders := []string{"Buy milk", "tomorrow 09:00", "normal", "home, errands", "none"}
	values := []string{item.Title, dueValue(item), string(item.Priority), strings.Join(item.Tags, ", "), string(item.RepeatRule)}
	if values[2] == "" {
		values[2] = string(todo.PriorityNormal)
	}
	if values[4] == "" {
		values[4] = string(todo.RepeatNone)
	}

	inputs := make([]textinput.Model, len(labels))
	for i := range labels {
		input := textinput.New()
		input.Prompt = labels[i] + ": "
		input.Placeholder = placeholders[i]
		input.SetValue(values[i])
		input.SetWidth(60)
		inputs[i] = input
	}

	notes := textarea.New()
	notes.Prompt = "Notes: "
	notes.Placeholder = "optional notes"
	notes.ShowLineNumbers = false
	notes.EndOfBufferCharacter = ' '
	notes.DynamicHeight = true
	notes.MinHeight = 1
	notes.MaxHeight = 6
	notes.SetWidth(72)
	notes.SetValue(item.Notes)
	return inputs, notes
}

func dueValue(item todo.Todo) string {
	if item.DueDate == "" {
		return ""
	}
	if item.DueTime == "" {
		return item.DueDate
	}
	return item.DueDate + " " + item.DueTime
}

func (m *Model) moveField(delta int) {
	if m.field == formNotes {
		m.notes.Blur()
	} else {
		m.inputs[m.field].Blur()
	}
	m.field += delta
	m.pick = 0
	if m.field < 0 {
		m.field = formNotes
	}
	if m.field > formNotes {
		m.field = 0
	}
}

func (m *Model) focusField() tea.Cmd {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	m.notes.Blur()
	m.clampPick()
	if m.field == formNotes {
		return m.notes.Focus()
	}
	return m.inputs[m.field].Focus()
}

func (m Model) hasSuggestions() bool {
	return len(m.suggestions()) > 0
}

func (m Model) suggestions() []suggestion {
	if len(m.inputs) == 0 {
		return nil
	}
	switch m.field {
	case formDue:
		return filteredSuggestions(dueSuggestions(time.Now()), m.inputs[m.field].Value())
	case formRepeat:
		return filteredSuggestions(repeatSuggestions(), m.inputs[m.field].Value())
	default:
		return nil
	}
}

func (m *Model) movePick(delta int) {
	suggestions := m.suggestions()
	if len(suggestions) == 0 {
		m.pick = 0
		return
	}
	m.pick += delta
	if m.pick < 0 {
		m.pick = len(suggestions) - 1
	}
	if m.pick >= len(suggestions) {
		m.pick = 0
	}
}

func (m *Model) clampPick() {
	suggestions := m.suggestions()
	if len(suggestions) == 0 || m.pick < 0 {
		m.pick = 0
		return
	}
	if m.pick >= len(suggestions) {
		m.pick = len(suggestions) - 1
	}
}

func (m *Model) applyPick() {
	suggestions := m.suggestions()
	if len(suggestions) == 0 {
		return
	}
	m.clampPick()
	m.inputs[m.field].SetValue(suggestions[m.pick].value)
}

func filteredSuggestions(suggestions []suggestion, input string) []suggestion {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return suggestions
	}
	for _, item := range suggestions {
		if strings.ToLower(item.value) == input {
			return suggestions
		}
	}
	filtered := make([]suggestion, 0, len(suggestions))
	for _, item := range suggestions {
		if strings.Contains(strings.ToLower(item.value), input) || strings.Contains(strings.ToLower(item.label), input) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func dueSuggestions(now time.Time) []suggestion {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	items := []suggestion{
		{value: "", label: "no due date"},
		{value: "today", label: "today, all day"},
		{value: "today 09:00", label: "today morning"},
		{value: "today 17:00", label: "today end of day"},
		{value: "tomorrow", label: "tomorrow, all day"},
		{value: "tomorrow 09:00", label: "tomorrow morning"},
	}
	for i := 2; i <= 7; i++ {
		date := day.AddDate(0, 0, i)
		items = append(items, suggestion{
			value: date.Format(time.DateOnly),
			label: date.Format("Monday, Jan 2"),
		})
	}
	return items
}

func repeatSuggestions() []suggestion {
	return []suggestion{
		{value: "none", label: "does not repeat"},
		{value: "daily", label: "every day"},
		{value: "weekly", label: "every week"},
		{value: "monthly", label: "every month"},
	}
}

func (m Model) saveForm() Model {
	title := strings.TrimSpace(m.inputs[0].Value())
	dueInput := strings.TrimSpace(m.inputs[1].Value())
	priorityInput := strings.TrimSpace(m.inputs[2].Value())
	tagsInput := strings.TrimSpace(m.inputs[3].Value())
	repeatInput := strings.TrimSpace(m.inputs[4].Value())
	notes := m.notes.Value()

	var due dateparse.Due
	if dueInput != "" {
		parsed, err := dateparse.ParseDue(dueInput, time.Now())
		if err != nil {
			m.err = err
			return m
		}
		due = parsed
	}
	priority, err := todo.NormalizePriority(priorityInput)
	if err != nil {
		m.err = err
		return m
	}
	repeat, err := todo.NormalizeRepeat(repeatInput)
	if err != nil {
		m.err = err
		return m
	}
	tags := todo.NormalizeTags([]string{tagsInput})

	if m.editingID == 0 {
		item, err := m.store.CreateTodo(m.ctx, todo.CreateParams{
			Title:      title,
			Notes:      notes,
			Priority:   priority,
			DueDate:    due.Date,
			DueTime:    due.Time,
			RepeatRule: repeat,
			Tags:       tags,
		})
		if err != nil {
			m.err = err
			return m
		}
		m.message = fmt.Sprintf("Created #%d", item.ID)
	} else {
		params := todo.UpdateParams{
			Title:      &title,
			Notes:      &notes,
			Priority:   &priority,
			DueDate:    &due.Date,
			DueTime:    &due.Time,
			RepeatRule: &repeat,
			Tags:       &tags,
		}
		item, err := m.store.UpdateTodo(m.ctx, m.editingID, params)
		if err != nil {
			m.err = err
			return m
		}
		m.message = fmt.Sprintf("Updated #%d", item.ID)
	}

	m.screen = screenList
	m.inputs = nil
	m.editingID = 0
	m.refresh()
	return m
}

func (m Model) listView() string {
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render("todo")
	fmt.Fprintf(&b, "%s  %s", header, lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(string(m.view)))
	if m.search != "" {
		fmt.Fprintf(&b, "  search:%s", m.search)
	}
	b.WriteString("\n")
	b.WriteString("1 open  2 today  3 tomorrow  4 this week  5 next week\n")
	b.WriteString("6 this month  7 next month  8 no due  9 done  0 archived\n\n")

	if m.err != nil {
		fmt.Fprintf(&b, "%s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error()))
	} else if m.message != "" {
		fmt.Fprintf(&b, "%s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(m.message))
	}

	if len(m.items) == 0 {
		b.WriteString("No todos.\n")
	} else {
		visible := max(5, m.height-10)
		end := min(len(m.items), m.offset+visible)
		for i := m.offset; i < end; i++ {
			prefix := "  "
			lineStyle := lipgloss.NewStyle()
			if i == m.cursor {
				prefix = "> "
				lineStyle = lineStyle.Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("8"))
			}
			fmt.Fprintf(&b, "%s%s\n", prefix, lineStyle.Render(renderTodoLine(m.items[i], m.width-2)))
		}
	}

	b.WriteString("\n")
	b.WriteString("j/k move  n new  e edit  enter done/reopen  a archive  u unarchive  x remove  / search  r refresh  q quit\n")
	return b.String()
}

func (m Model) formView() string {
	var b strings.Builder
	title := "New todo"
	if m.editingID != 0 {
		title = fmt.Sprintf("Edit #%d", m.editingID)
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render(title))
	b.WriteString("\n\n")
	if m.err != nil {
		fmt.Fprintf(&b, "%s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error()))
	}
	for i, input := range m.inputs {
		line := input.View()
		if i == m.field {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
		if i == m.field {
			b.WriteString(m.suggestionsView())
		}
	}
	notes := m.notes.View()
	if m.field == formNotes {
		notes = lipgloss.NewStyle().Bold(true).Render(notes)
	}
	b.WriteString(notes)
	if !strings.HasSuffix(notes, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.hasSuggestions() {
		b.WriteString("up/down pick  enter apply/next  tab next  ctrl+s save  esc cancel\n")
	} else if m.field == formNotes {
		b.WriteString("enter newline  tab next  ctrl+s save  esc cancel\n")
	} else {
		b.WriteString("tab next  enter advance/save  ctrl+s save  esc cancel\n")
	}
	return b.String()
}

func (m Model) suggestionsView() string {
	suggestions := m.suggestions()
	if len(suggestions) == 0 {
		if m.field == formDue || m.field == formRepeat {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  no matching suggestions") + "\n"
		}
		return ""
	}

	var b strings.Builder
	limit := min(len(suggestions), 7)
	for i := range limit {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		if i == m.pick {
			prefix = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
		}
		value := suggestions[i].value
		if value == "" {
			value = "(blank)"
		}
		fmt.Fprintf(&b, "%s%s\n", prefix, style.Render(fmt.Sprintf("%-16s %s", value, suggestions[i].label)))
	}
	if len(suggestions) > limit {
		fmt.Fprintf(&b, "  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("+%d more, keep typing to filter", len(suggestions)-limit)))
	}
	return b.String()
}

func (m Model) searchView() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render("Search"))
	b.WriteString("\n\n")
	b.WriteString(m.inputs[0].View())
	b.WriteString("\n\nenter apply  esc cancel\n")
	return b.String()
}

func (m Model) confirmDeleteView() string {
	return fmt.Sprintf("Remove #%d permanently?\n\nThis cannot be undone.\n\nPress y to remove, n or esc to cancel.\n", m.confirmID)
}

func renderTodoLine(item todo.Todo, width int) string {
	status := "open"
	if item.Status == todo.StatusDone {
		status = "done"
	}
	if item.IsArchived() {
		status = "archived"
	}
	tags := ""
	if len(item.Tags) > 0 {
		tags = " [" + strings.Join(item.Tags, ", ") + "]"
	}
	line := fmt.Sprintf("#%-4d %-8s %-8s %-16s %s%s", item.ID, item.Priority, status, item.DueLabel(), item.Title, tags)
	if width > 0 && len(line) > width {
		return line[:max(0, width-3)] + "..."
	}
	return line
}
