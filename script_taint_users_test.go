package jactionlint

import "testing"

// TestJQKindsUsers: for the user objects of GitHub (user, author, committer, assignees, reviewers, sender, owner, merged_by
// ...), `.login` is a login, the other text of the person (name, email, bio, company, location, blog) is free text, and the
// object itself holds both: the strictest of the two.
func TestJQKindsUsers(t *testing.T) {
	const login = kindName
	const person = kindName | kindPerson
	tests := []struct {
		filter string
		want   int
	}{
		{".user.login", login},
		{".author.login", login},
		{".committer.login", login},
		{".assignee.login", login},
		{".sender.login", login},
		{".owner.login", login},
		{".merged_by.login", login},
		{".closed_by.login", login},
		{".user.id", login},
		{".user.type", login},
		{".user.html_url", login},
		{".user.name", person},
		{".user.email", person},
		{".user.bio", person},
		{".user.company", person},
		{".user.location", person},
		{".user.blog", person},
		{".user", person},
		{".author", person},
		{".author.name", person},
		{".committer.name", person},
		{".assignee.name", person},
		{".creator.name", person},
		{".actor", person},
		{".owner", person},
		{".merged_by.name", person},
		{".assignees[].login", login | kindMulti},
		{".assignees[].name", person | kindMulti},
		{".requested_reviewers[].login", login | kindMulti},
		{".requested_reviewers[].name", person | kindMulti},
		{".user.login, .user.name", person},
		{".user | .name", person},
		{".user.login, .assignee.login", login},
		{".title", kindText},
		{".user.login, .title", login | kindText},
	}
	for _, tc := range tests {
		if got := jqKinds(tc.filter); got != tc.want {
			t.Errorf("jqKinds(%q) = %b, want %b", tc.filter, got, tc.want)
		}
	}
	for _, tc := range []struct {
		src        taintSource
		name, free bool
	}{
		{taintSource{kinds: kindName}, true, true},
		{taintSource{kinds: kindName | kindPerson}, true, false},
		{taintSource{kinds: kindText}, false, false},
		{taintSource{kinds: kindName | kindText}, false, false},
	} {
		if got := tc.src.nameOnly(); got != tc.name {
			t.Errorf("nameOnly(%b) = %v, want %v", tc.src.kinds, got, tc.name)
		}
		if got := tc.src.refOrLoginOnly(); got != tc.free {
			t.Errorf("refOrLoginOnly(%b) = %v, want %v", tc.src.kinds, got, tc.free)
		}
	}
}
