package utfkenall_test

import (
	"fmt"
	"testing"

	"github.com/memwey/utfkenall"
)

func TestAddressFormatting(t *testing.T) {
	tests := []struct {
		code        string
		japanese    string
		english     string
		explanation string
	}{
		{
			code:     "100-0001",
			japanese: "東京都千代田区千代田",
			english:  "Chiyoda, Chiyoda-ku, Tokyo",
		},
		{
			code:        "060-0000",
			japanese:    "北海道札幌市中央区",
			english:     "Sapporo-shi Chuo-ku, Hokkaido",
			explanation: "no town, so neither rendering leaves a gap",
		},
	}
	for _, tt := range tests {
		a, err := utfkenall.Lookup(tt.code)
		if err != nil {
			t.Fatalf("%s: %v", tt.code, err)
		}
		if got := a.String(); got != tt.japanese {
			t.Errorf("%s String() = %q, want %q", tt.code, got, tt.japanese)
		}
		if got := a.English(); got != tt.english {
			t.Errorf("%s English() = %q, want %q", tt.code, got, tt.english)
		}
	}
}

func TestNameString(t *testing.T) {
	n := utfkenall.Name{Kanji: "千代田区", Kana: "チヨダク", Romaji: "Chiyoda-ku"}
	if got := n.String(); got != "千代田区" {
		t.Errorf("String() = %q, want the kanji form", got)
	}
	if got := fmt.Sprint(n); got != "千代田区" {
		t.Errorf("fmt.Sprint = %q, want the kanji form", got)
	}
}

func ExampleLookup() {
	addr, err := utfkenall.Lookup("100-0001")
	if err != nil {
		panic(err)
	}
	fmt.Println(addr)
	fmt.Println(addr.English())
	fmt.Println(addr.Town.Kana)
	// Output:
	// 東京都千代田区千代田
	// Chiyoda, Chiyoda-ku, Tokyo
	// チヨダ
}

func ExampleLookupAll() {
	// A handful of codes straddle a prefecture border.
	addrs, err := utfkenall.LookupAll("498-0000")
	if err != nil {
		panic(err)
	}
	for _, a := range addrs {
		fmt.Println(a)
	}
	// Output:
	// 愛知県弥富市
	// 三重県桑名郡木曽岬町
}
