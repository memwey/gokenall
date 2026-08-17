package main

import "testing"

func TestCleanTown(t *testing.T) {
	tests := []struct {
		name  string
		kanji string
		kana  string
		want  cleanedTown
	}{
		{
			name:  "plain town is left alone",
			kanji: "千代田",
			kana:  "チヨダ",
			want:  cleanedTown{Kanji: "千代田", Kana: "チヨダ"},
		},
		{
			name:  "以下に掲載がない場合 is a placeholder",
			kanji: "以下に掲載がない場合",
			kana:  "イカニケイサイガナイバアイ",
		},
		{
			name:  "の次に番地がくる場合 is a placeholder",
			kanji: "境町の次に番地がくる場合",
			kana:  "サカイマチノツギニバンチガクルバアイ",
		},
		{
			name:  "村一円 is a placeholder",
			kanji: "利島村一円",
			kana:  "トシマムライチエン",
		},
		{
			name:  "一円 on its own is a real town in 滋賀県犬上郡多賀町",
			kanji: "一円",
			kana:  "イチエン",
			want:  cleanedTown{Kanji: "一円", Kana: "イチエン"},
		},
		{
			name:  "その他 says nothing and is dropped entirely",
			kanji: "岩倉上蔵町（その他）",
			kana:  "イワクラアグラチョウ（ソノタ）",
			want:  cleanedTown{Kanji: "岩倉上蔵町", Kana: "イワクラアグラチョウ"},
		},
		{
			name:  "地階・階層不明 is dropped entirely",
			kanji: "阿倍野筋あべのハルカス（地階・階層不明）",
			kana:  "アベノスジアベノハルカス（チカイ・カイソウフメイ）",
			want:  cleanedTown{Kanji: "阿倍野筋あべのハルカス", Kana: "アベノスジアベノハルカス"},
		},
		{
			name:  "を除く exclusions are dropped entirely",
			kanji: "阿倍野筋（次のビルを除く）",
			kana:  "アベノスジ（ツギノビルヲノゾク）",
			want:  cleanedTown{Kanji: "阿倍野筋", Kana: "アベノスジ"},
		},
		{
			name:  "a floor is kept as a note",
			kanji: "阿倍野筋あべのハルカス（６０階）",
			kana:  "アベノスジアベノハルカス（６０カイ）",
			want: cleanedTown{
				Kanji: "阿倍野筋あべのハルカス",
				Kana:  "アベノスジアベノハルカス",
				Note:  "６０階",
			},
		},
		{
			// The old library expanded this into six records. That invents zip
			// code entries Japan Post never published, so the range stays a note.
			name:  "a 丁目 range is kept as a note, not expanded",
			kanji: "天神橋（１〜６丁目）",
			kana:  "テンジンバシ（１−６チョウメ）",
			want: cleanedTown{
				Kanji: "天神橋",
				Kana:  "テンジンバシ",
				Note:  "１〜６丁目",
			},
		},
		{
			name:  "a list of sub-place names is kept as a note",
			kanji: "大通西（１〜１９丁目）",
			kana:  "オオドオリニシ（１−１９チョウメ）",
			want: cleanedTown{
				Kanji: "大通西",
				Kana:  "オオドオリニシ",
				Note:  "１〜１９丁目",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanTown(tt.kanji, tt.kana); got != tt.want {
				t.Errorf("cleanTown(%q, %q)\n got %#v\nwant %#v", tt.kanji, tt.kana, got, tt.want)
			}
		})
	}
}

func TestStripRomeNote(t *testing.T) {
	tests := []struct {
		in        string
		want      string
		wantWhole bool
	}{
		{"MARUNOCHI", "MARUNOCHI", true},
		{"ODORINISHI(1-19-CHOME)", "ODORINISHI", true},
		{"NISHI19-JOMINAMI(35-38.41.42-CHOME)", "NISHI19-JOMINAMI", true},
		// Cut inside the annotation: the name itself survived.
		{"OMBETSUCHO OMBETSUGENYAKISEN(FUTAMA", "OMBETSUCHO OMBETSUGENYAKISEN", true},
		// Cut before any annotation began: the name is a fragment.
		{"HIGASHIASAHIKAWACHO HIGASHISAKURAOK", "HIGASHIASAHIKAWACHO HIGASHISAKURAOK", false},
		{"3.431-12.443-6.608-2.641-8.814.842-", "3.431-12.443-6.608-2.641-8.814.842-", false},
	}
	for _, tt := range tests {
		got, whole := stripRomeNote(tt.in)
		if got != tt.want || whole != tt.wantWhole {
			t.Errorf("stripRomeNote(%q) = %q, %v; want %q, %v", tt.in, got, whole, tt.want, tt.wantWhole)
		}
	}
}

func TestStripRomeNoteKanji(t *testing.T) {
	tests := []struct {
		in        string
		want      string
		wantWhole bool
	}{
		{"丸の内", "丸の内", true},
		{"大通西（１〜１９丁目）", "大通西", true},
		{"東旭川町豊田（１〜９番地）", "東旭川町豊田", true},
		// 17 full-width characters with the annotation cut open.
		{"東旭川町東桜岡（３０〜４９９番地", "東旭川町東桜岡", true},
		// 17 characters and no annotation at all: a fragment.
		{"３、４３１−１２、４４３−６、６０", "３、４３１−１２、４４３−６、６０", false},
	}
	for _, tt := range tests {
		got, whole := stripRomeNoteKanji(tt.in)
		if got != tt.want || whole != tt.wantWhole {
			t.Errorf("stripRomeNoteKanji(%q) = %q, %v; want %q, %v", tt.in, got, whole, tt.want, tt.wantWhole)
		}
	}
}
