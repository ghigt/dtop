package main

type ItemPair struct {
	Com string
	Def string
}

type Page struct {
	Header string
	Info   string
	Body   []ItemPair
	Footer string
}

var help = Page{
	Header: `
Help of "pot" command:
`,
	Info: `
`,
	Body: []ItemPair{
		{"<arrow up>", "scroll up"},
		{"<arrow down>", "scroll down"},
		{"<space>", "select/unselect container"},
		{"u", "unselect all containers"},
		{"q", "quit"},
		{"h", "prints this help"},
		{"a", "show/hide processes on selected containers"},
		{"A", "show/hide processes on all containers"},
		{"k", "kill selected containers"},
		{"s", "start selected containers"},
		{"S", "stop selected containers"},
		{"r", "remove selected containers"},
		{"i", "view information about current container"},
		{"p", "pause selected containers"},
		{"P", "unpause selected containers"},
		{"1", "sort by name"},
		{"2", "sort by image"},
		{"3", "sort by id"},
		{"4", "sort by command"},
		{"5", "sort by uptime"},
		{"6", "sort by status"},
		{"7", "sort by %CPU"},
		{"8", "sort by %RAM"},
		{"I", "revert current sort"},
	},
	Footer: `
Press 'h' to return.
`,
}
