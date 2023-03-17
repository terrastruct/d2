package e2etests

import (
	_ "embed"
	"testing"

	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
)

func testThemes(t *testing.T) {
	tcs := []testCase{
		{
			name:    "terminal",
			themeID: d2themescatalog.Terminal.ID,
			script: `
network: {
  cell tower: {
		satellites: {
			shape: stored_data
      style.multiple: true
		}

		transmitter

		satellites -> transmitter: send
		satellites -> transmitter: send
		satellites -> transmitter: send
  }

  online portal: {
    ui: { shape: hexagon }
  }

  data processor: {
    storage: {
      shape: cylinder
      style.multiple: true
    }
  }

  cell tower.transmitter -> data processor.storage: phone logs
}

user: {
  shape: person
  width: 130
}

user -> network.cell tower: make call
user -> network.online portal.ui: access {
  style.stroke-dash: 3
}

api server -> network.online portal.ui: display
api server -> logs: persist
logs: { shape: page; style.multiple: true }

network.data processor -> api server
users: {
	shape: sql_table
	id: int
	name: string
	email: string
	password: string
	last_login: datetime
}

products: {
	shape: class
	id: int
	price: decimal
	sku: string
	name: string
}
markdown: |md
  # A tale
  - of
  - two cities
|
`,
		},
		{
			name:    "terminal_grayscale",
			themeID: d2themescatalog.TerminalGrayscale.ID,
			script: `
network: {
  cell tower: {
		satellites: {
			shape: stored_data
      style.multiple: true
		}

		transmitter

		satellites -> transmitter: send
		satellites -> transmitter: send
		satellites -> transmitter: send
  }

  online portal: {
    ui: { shape: hexagon }
  }

  data processor: {
    storage: {
      shape: cylinder
      style.multiple: true
    }
  }

  cell tower.transmitter -> data processor.storage: phone logs
}

user: {
  shape: person
  width: 130
}

user -> network.cell tower: make call
user -> network.online portal.ui: access {
  style.stroke-dash: 3
}

api server -> network.online portal.ui: display
api server -> logs: persist
logs: { shape: page; style.multiple: true }

network.data processor -> api server
`,
		},
	}

	runa(t, tcs)
}
