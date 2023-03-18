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
code: |go
package main

import (
	"fmt"
)

type City struct {
	Name       string
	Population int
}

func tellTale(city1, city2 City) {
	fmt.Printf("There were two cities, %s and %s.\n", city1.Name, city2.Name)
	fmt.Printf("%s had a population of %d.\n", city1.Name, city1.Population)
	fmt.Printf("%s had a population of %d.\n", city2.Name, city2.Population)
	fmt.Println("Their tales were intertwined, and their people shared many adventures.")
}

func main() {
	city1 := City{Name: "CityA", Population: 1000000}
	city2 := City{Name: "CityB", Population: 1200000}

	tellTale(city1, city2)
}
|

markdown -> code
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
