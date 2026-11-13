package ui

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTrieNodeJSON(t *testing.T) {
	Convey("Setup", t, func() {
		node := &TrieNodeJSON{
			ID:          "root",
			TokenPrefix: "R",
			Token:       "R00",
			Action:      "enter",
			Count:       3,
			Children: []*TrieNodeJSON{
				{ID: "child", TokenPrefix: "R0"},
			},
		}

		Convey("When marshaled to JSON", func() {
			raw, err := json.Marshal(node)

			Convey("The wire contract matches exactly", func() {
				So(err, ShouldBeNil)
				So(string(raw), ShouldEqual,
					`{"id":"root","prefix":"R","token":"R00","action":"enter","count":3,"children":[{"id":"child","prefix":"R0"}]}`,
				)
			})
		})

		Convey("When omitempty fields are zero", func() {
			bare := &TrieNodeJSON{ID: "n", TokenPrefix: "p"}
			raw, err := json.Marshal(bare)

			Convey("Token Action Count Children are omitted", func() {
				So(err, ShouldBeNil)
				So(string(raw), ShouldEqual, `{"id":"n","prefix":"p"}`)
			})
		})
	})
}

func TestTrieBranchJSON(t *testing.T) {
	Convey("Setup", t, func() {
		branch := TrieBranchJSON{
			Hash:       "abc",
			Depth:      2,
			Visits:     4,
			MeanEdge:   0.5,
			Confidence: 0.75,
			Policy:     "enter",
		}

		Convey("When marshaled to JSON", func() {
			raw, err := json.Marshal(branch)

			Convey("The wire contract matches exactly", func() {
				So(err, ShouldBeNil)
				So(string(raw), ShouldEqual,
					`{"hash":"abc","depth":2,"visits":4,"meanEdge":0.5,"confidence":0.75,"policy":"enter"}`,
				)
			})
		})
	})
}

func TestCognitionTreeExport(t *testing.T) {
	Convey("Setup", t, func() {
		export := CognitionTreeExport{
			Keys: []string{"a", "b"},
			Root: &TrieNodeJSON{ID: "root", TokenPrefix: ""},
			Branches: []TrieBranchJSON{
				{Hash: "h1", Depth: 1, Visits: 1, MeanEdge: 0, Confidence: 1, Policy: "hold"},
			},
		}

		Convey("When marshaled to JSON", func() {
			raw, err := json.Marshal(export)

			Convey("The wire contract matches exactly", func() {
				So(err, ShouldBeNil)
				So(string(raw), ShouldEqual,
					`{"keys":["a","b"],"root":{"id":"root","prefix":""},"branches":[{"hash":"h1","depth":1,"visits":1,"meanEdge":0,"confidence":1,"policy":"hold"}]}`,
				)
			})
		})

		Convey("When Keys and Root are empty", func() {
			empty := CognitionTreeExport{Branches: []TrieBranchJSON{}}
			raw, err := json.Marshal(empty)

			Convey("Keys and Root are omitted and Branches is an empty array", func() {
				So(err, ShouldBeNil)
				So(string(raw), ShouldEqual, `{"branches":[]}`)
			})
		})

		Convey("When unmarshaled from wire JSON", func() {
			const wire = `{"keys":["k"],"root":{"id":"r","prefix":"p"},"branches":[{"hash":"h","depth":3,"visits":9,"meanEdge":0.25,"confidence":0.5,"policy":"exit"}]}`
			var got CognitionTreeExport
			err := json.Unmarshal([]byte(wire), &got)

			Convey("Fields round-trip exactly", func() {
				So(err, ShouldBeNil)
				So(len(got.Keys), ShouldEqual, 1)
				So(got.Keys[0], ShouldEqual, "k")
				So(got.Root, ShouldNotBeNil)
				So(got.Root.ID, ShouldEqual, "r")
				So(got.Root.TokenPrefix, ShouldEqual, "p")
				So(len(got.Branches), ShouldEqual, 1)
				So(got.Branches[0].Hash, ShouldEqual, "h")
				So(got.Branches[0].Depth, ShouldEqual, 3)
				So(got.Branches[0].Visits, ShouldEqual, 9)
				So(got.Branches[0].MeanEdge, ShouldEqual, 0.25)
				So(got.Branches[0].Confidence, ShouldEqual, 0.5)
				So(got.Branches[0].Policy, ShouldEqual, "exit")
			})
		})
	})
}
