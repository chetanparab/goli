package bookmarks

import "testing"

const netscape = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>Bookmarks</TITLE><H1>Bookmarks</H1>
<DL><p>
  <DT><H3 ADD_DATE="1">Bookmarks Bar</H3>
  <DL><p>
    <DT><A HREF="https://github.com/acme/app/pulls" ADD_DATE="1">Pull requests · acme/app</A>
    <DT><H3>Work</H3>
    <DL><p>
      <DT><A HREF="https://meet.google.com/abc-defg-hij">Daily stand-up</A>
      <DT><A HREF="javascript:void(0)">Bookmarklet</A>
    </DL><p>
  </DL><p>
  <DT><A HREF="https://news.ycombinator.com/">Hacker News</A>
</DL><p>`

func TestNetscape(t *testing.T) {
	bs, err := Parse([]byte(netscape))
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 3 {
		t.Fatalf("got %d bookmarks: %+v", len(bs), bs)
	}
	if bs[0].Folder != "Bookmarks Bar" || bs[1].Folder != "Bookmarks Bar/Work" || bs[2].Folder != "" {
		t.Errorf("folders: %q %q %q", bs[0].Folder, bs[1].Folder, bs[2].Folder)
	}
	if bs[1].Title != "Daily stand-up" || Key(bs[1]) != "daily-stand-up" {
		t.Errorf("title %q key %q", bs[1].Title, Key(bs[1]))
	}
	if Key(bs[0]) != "pull-requests-acme" || Key(bs[2]) != "hacker-news" {
		t.Errorf("keys %q %q", Key(bs[0]), Key(bs[2]))
	}
}

func TestChrome(t *testing.T) {
	data := `{"roots":{"bookmark_bar":{"type":"folder","name":"Bookmarks bar","children":[
	  {"type":"url","name":"Grafana","url":"https://grafana.example.com/d/prod"},
	  {"type":"folder","name":"Docs","children":[{"type":"url","name":"","url":"https://docs.example.com/"}]}]},
	  "other":{"type":"folder","name":"Other bookmarks","children":[{"type":"url","name":"Local","url":"chrome://settings"}]}}}`
	bs, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 || bs[1].Folder != "Bookmarks bar/Docs" || bs[1].Title != "docs.example.com" || Key(bs[1]) != "docs-example" {
		t.Errorf("%+v keys %q", bs, Key(bs[1]))
	}
	if _, err := Parse([]byte("hello")); err == nil {
		t.Error("garbage accepted")
	}
}
