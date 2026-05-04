on the wiki page for Begum it says she was "Member of Parliament
for Scarborough Southwest". Scarborough Southwest is the riding.
The user is pointing out that for Doly Begum, the Wikipedia page shows her current role as "Member of Parliament for Scarborough Southwest" (federal MP, since 2026), and the riding we should be storing is "Scarborough Southwest".

Looking at my current code:


isProvincial := strings.Contains(label, "legislative assembly") ||
    strings.Contains(label, "provincial parliament") ||
    strings.Contains(label, "house of assembly") ||
    strings.Contains(label, "national assembly")
if !isProvincial {
    return
}
This currently SKIPS "Member of Parliament" rows (the federal ones), but the user is saying we SHOULD use the riding from those rows too - because the riding name "Scarborough Southwest" is the same whether it's the federal MP role or the provincial MPP role.

I see now - both the federal and provincial roles list the same riding name, so my filter is unnecessarily excluding valid riding data. The test was failing because I was skipping the federal MP row when I should have been extracting the riding from it. I'm realizing my test case might be the actual problem here—I hardcoded "Scarborough Southwest (federal)" to distinguish it from the provincial riding, but in reality both the federal and provincial ridings share the same name. The user's point is that since we're matching against provincial voting records, we should accept the riding name from whichever row has it, whether that's the federal or provincial row, because the riding name itself is what matters for the lookup.

But there's a real edge case I need to handle: when federal and provincial ridings have different names entirely, like "Simcoe North" versus "Barrie-Innisfil", I'd want to use the provincial one for a provincial vote matching system. So the question is whether to prefer the provincial row specifically, or just use whatever riding name we find first.

Looking at my current implementation, I'm filtering for provincial legislatures by checking for keywords like "legislative assembly" and "provincial parliament" in the label. The user's point about Doly Begum's case suggests this approach is already working—the infobox shows the federal role at the top, but my code correctly identifies and uses the provincial riding row instead.

However, I'm now wondering if I should also consider extracting riding information from federal "Member of Parliament" rows as a fallback, since those riding names still represent the actual geographic area, even if they're technically federal ridings rather than provincial ones. should also handle the case where there's ONLY a "Member of Parliament" row and no provincial variant—maybe that's what they're getting at. Let me think about whether my current approach covers that scenario or if I need to adjust the logic to use federal riding as a fallback. I'm realizing the simplest approach is to just accept any "for [riding]" pattern regardless of whether it's from a provincial or federal office row, since we're primarily enriching provincial member records and having any riding is better than none. I'm realizing the user might be questioning whether my provincial filter is too restrictive—maybe they're wondering if there are Canadian provinces with different legislative titles I haven't accounted for. But actually, I've covered all the main ones: MLA, MPP, MHA, and MNA. The only thing my filter would skip is "Member of Parliament" which is federal, so that's the correct behavior. For Doly Begum's case in Ontario, "Member of Provincial Parliament" is explicitly included in my check, so it should work.

I think the real issue might be that I should simplify this entirely—instead of trying to be clever with fallbacks, I should just take the last "for [riding]" row found in any legislature context, which would naturally handle edge cases without overthinking the filter logic. I'm realizing I should just take the first "for [riding]" match regardless of whether it's provincial or federal, since the rows are already ordered by current role. For someone like Begum who has both federal and provincial positions, the ridings usually match anyway, so the order shouldn't matter much. The real edge case would be former provincial members no longer in the API or very new ones not yet indexed, but those are probably rare enough that processing in document order is fine.

I'll implement a two-pass approach: first look for provincial ridings, then fall back to any "for [riding]" row if needed.

Updating the code...

The user is pointing out that the "Member of Parliament for Scarborough Southwest" row in the infobox also carries the riding — and that row should be usable as a fallback when no provincial legislature row is found. Let me update the filter to do a two-pass: prefer provincial rows, fall back to any "for [riding]" row.

Read wiki_lookup.go (lines 203-262)
I need to update the office header section to:

Still prefer provincial legislature rows (set info.riding directly)
For federal "Member of Parliament" rows, store in a fallback variable
After the Each loop, if info.riding == "", use the fallback
Let me restructure the fetchArticle function to use a local fallbackRiding variable.

★ Insight ─────────────────────────────────────

Federal and provincial ridings in Canada often share a name (like "Scarborough Southwest") because provincial ridings were carved from federal ones, so the fallback is usually still correct
The two-pass approach (prefer provincial, fall back to any legislature) is better than skipping non-provincial rows entirely, since some members' articles only prominently show their current federal role
─────────────────────────────────────────────────
Edit wiki_lookup.go
Added 15 lines
Now update the test to assert the fallback actually works:

Edit wiki_lookup_test.go
Added 24 lines
Bash Run all wiki lookup tests
IN
go test ./internal/scraper/provincial/... -run "TestWikiLookup" -v 2>&1

OUT
Exit code 1
=== RUN   TestWikiLookup_BySurname
--- PASS: TestWikiLookup_BySurname (0.02s)
=== RUN   TestWikiLookup_AccentFolding
--- PASS: TestWikiLookup_AccentFolding (0.01s)
=== RUN   TestWikiLookup_ByFullName
--- PASS: TestWikiLookup_ByFullName (0.01s)
=== RUN   TestWikiLookup_Disambiguation
--- PASS: TestWikiLookup_Disambiguation (0.01s)
=== RUN   TestWikiLookup_UnknownName
--- PASS: TestWikiLookup_UnknownName (0.01s)
=== RUN   TestWikiLookup_ArticleCached
--- PASS: TestWikiLookup_ArticleCached (0.01s)
=== RUN   TestWikiLookup_CategoryFetchFailure
--- PASS: TestWikiLookup_CategoryFetchFailure (0.01s)
=== RUN   TestWikiLookup_MultipleCategoriesMerged
--- PASS: TestWikiLookup_MultipleCategoriesMerged (0.01s)
=== RUN   TestWikiLookup_NewProvincialWikiLookup
--- PASS: TestWikiLookup_NewProvincialWikiLookup (0.00s)
=== RUN   TestWikiLookup_PartyQualifierStripped
--- PASS: TestWikiLookup_PartyQualifierStripped (0.01s)
=== RUN   TestWikiLookup_RidingFromColspan2Header
--- PASS: TestWikiLookup_RidingFromColspan2Header (0.01s)
=== RUN   TestWikiLookup_RidingFallsBackToFederalRow
    wiki_lookup_test.go:332: riding="", want "Scarborough Southwest" (should fall back to federal row)
--- FAIL: TestWikiLookup_RidingFallsBackToFederalRow (0.01s)
FAIL
FAIL	github.com/philspins/opendocket/internal/scraper/provincial	0.134s
FAIL
The test for the fallback case is failing. Let me check the logic again.

The issue: in the test's HTML, the <th> text is "Member of Parliament for Scarborough Southwest". Let me check if my condition strings.Contains(label, " for ") is matching.

The label would be "member of parliament for scarborough southwest" (lowercase). Does it contain " for "? Yes! And isProvincial would be false. So fallbackRiding should be set to "Scarborough Southwest".

Wait, but the test HTML has:


<tr><th colspan="2" class="infobox-header"><a href="/wiki/MP">Member of Parliament</a><br />for <a href="/wiki/Riding">Scarborough Southwest</a></th></tr>
The th.Text() on this would give "Member of Parliament\nfor Scarborough Southwest" (with a newline from <br/>). After strings.TrimSpace, it only removes leading/trailing whitespace, not internal newlines.

So label would be "member of parliament\nfor scarborough southwest". The issue is that my condition is looking for " for " (with spaces), but the actual text has a newline before "for" — so it doesn't match the pattern.

The fix is to normalize the whitespace in the label so that newlines become spaces. I'll use strings.Fields to split on any whitespace and rejoin with single spaces, which handles the \nfor problem cleanly.

The <br/> between "Member of Parliament" and "for" becomes a newline in .Text(), so " for " doesn't match "\nfor ". Need to normalize whitespace in the label.

Edit wiki_lookup.go
Added 1 line
Bash Run wiki lookup tests
IN
go test ./internal/scraper/provincial/... -run "TestWikiLookup" -v 2>&1

OUT
Exit code 1
=== RUN   TestWikiLookup_BySurname
--- PASS: TestWikiLookup_BySurname (0.02s)
=== RUN   TestWikiLookup_AccentFolding
--- PASS: TestWikiLookup_AccentFolding (0.01s)
=== RUN   TestWikiLookup_ByFullName
--- PASS: TestWikiLookup_ByFullName (0.01s)
=== RUN   TestWikiLookup_Disambiguation
--- PASS: TestWikiLookup_Disambiguation (0.01s)
=== RUN   TestWikiLookup_UnknownName
--- PASS: TestWikiLookup_UnknownName (0.01s)
=== RUN   TestWikiLookup_ArticleCached
--- PASS: TestWikiLookup_ArticleCached (0.01s)
=== RUN   TestWikiLookup_CategoryFetchFailure
--- PASS: TestWikiLookup_CategoryFetchFailure (0.01s)
=== RUN   TestWikiLookup_MultipleCategoriesMerged
--- PASS: TestWikiLookup_MultipleCategoriesMerged (0.01s)
=== RUN   TestWikiLookup_NewProvincialWikiLookup
--- PASS: TestWikiLookup_NewProvincialWikiLookup (0.00s)
=== RUN   TestWikiLookup_PartyQualifierStripped
--- PASS: TestWikiLookup_PartyQualifierStripped (0.01s)
=== RUN   TestWikiLookup_RidingFromColspan2Header
--- PASS: TestWikiLookup_RidingFromColspan2Header (0.01s)
=== RUN   TestWikiLookup_RidingFallsBackToFederalRow
    wiki_lookup_test.go:332: riding="", want "Scarborough Southwest" (should fall back to federal row)
--- FAIL: TestWikiLookup_RidingFallsBackToFederalRow (0.01s)
FAIL
FAIL	github.com/philspins/opendocket/internal/scraper/provincial	0.135s
FAIL
The test is still failing. Let me think more carefully.

In the test HTML for the federal-only case:


<tr><th colspan="2" class="infobox-header"><a href="/wiki/MP">Member of Parliament</a><br />for <a href="/wiki/Riding">Scarborough Southwest</a></th></tr>
After normalizing: label = "member of parliament for scarborough southwest"

Does this contain " for "? Yes, it does! So the " for " check passes. Then:

isProvincial check: does it contain "legislative assembly"? No.
So I'm checking for "provincial parliament", "house of assembly", and "national assembly" — all return false, so isProvincial = false. The fallbackRiding should be set to "Scarborough Southwest", and at the end when info.riding is empty, it should get populated with that fallback value. But the test is returning an empty riding, which means something's breaking in the logic. Let me add some debugging to trace where this is going wrong.

Looking back at the test setup, I see the makeWikiServer function is creating an HTTP mock that serves the category page with links to articles. The category handler builds HTML with list items containing links to each article path.

When the lookup processes "Doly Begum" from that link, it normalizes the name and extracts the surname "begum" as the key. This gets stored in the byNormSurname map, so when I later call lookup("Begum"), it should find the entry by matching against that normalized surname key.

Now I'm fetching the article and parsing the infobox table to extract the relevant information like party affiliation. I see the issue now—I'm checking if the riding text is empty and returning early, but I'm not actually assigning it to info.riding before that check. The logic flow is stopping before it reaches the assignment, which is why the test shows an empty riding value even though I'm extracting the text correctly. I'm checking how goquery's Find method works within a selected element — it searches within that element's descendants. So when I call th.Find("a"), I get both the "Member of Parliament" and "Scarborough Southwest" links, and .Last() correctly grabs the second one. Now I'm tracing through the second row to see where the parsing might be going wrong with the Party field. The closure is capturing fallbackRiding correctly by reference, so that should work. Let me add some debug logging to trace what's actually happening, or maybe I should review the exact edit I made to see if something got changed unexpectedly.

Let me read the current state of the function to debug the failing test:

Read wiki_lookup.go (lines 206-285)
The code looks correct. Let me trace through the failing test more carefully.

The test HTML is:


<table class="infobox vcard">
<tr><th colspan="2" class="infobox-header"><a href="/wiki/MP">Member of Parliament</a><br />for <a href="/wiki/Riding">Scarborough Southwest</a></th></tr>
<tr><th scope="row">Party</th><td><a href="/wiki/Party">Liberal</a></td></tr>


</table>
So doc.Find("table.infobox tr") should match 2 rows. For the first row with the colspan header, row.Find("th") gets the header element and row.Find("td").First() returns nothing, so we skip the first branch and extract the label from the header text after normalizing it to "member of parliament for scarborough southwest".

Now I'm realizing the actual text content from that header — when goquery calls .Text() on the <th> element containing the link, the <br /> tag creates a newline character in the output, so the full text becomes "Member of Parliament\nfor Scarborough Southwest" rather than being concatenated without spacing.

When I apply strings.Fields() to split on whitespace and then rejoin with spaces, it normalizes everything to lowercase and produces "member of parliament for scarborough southwest" — which does contain the substring " for ", so the check would pass.

Looking at the HTML structure, .Last() correctly grabs the second anchor tag containing "Scarborough Southwest", and since there's no provincial parliament indicator, isProvincial stays false while fallbackRiding gets set properly. After the loop, the empty info.riding gets filled with the fallback value.

The logic seems sound, so I need to check what the actual test is expecting — maybe the issue is in how the lookup itself is being performed. The test server setup looks correct—when makeWikiServer registers the article at /wiki/Doly_Begum and lookup.baseURL is set to srv.URL, the article URL constructed in ensureLoaded should match perfectly. Let me trace through the test itself to see what's actually being tested... I'm verifying the category page handler is properly registered, then tracing through how the link to Doly Begum gets processed—checking that the href parsing, prefix validation, and name normalization all work correctly to extract the candidate's name.

The lookup function retrieves the stored entry by normalized surname, fetches the article HTML from the server, and now I'm stepping through how the article rows get parsed.

Looking at the first row in the infobox table—it's a header row with colspan="2" containing "Member of Parliament for Scarborough Southwest". When I search for <td> elements it comes up empty, so the code correctly branches into the colspan header handling. The text extraction pulls the content across the <br/> tag as a single string.

After normalizing with strings.Fields and strings.Join, the label becomes "member of parliament for scarborough southwest" in lowercase. The check for " for " in the label passes, confirming this is a riding header. The last <a> element in the header gives me "Scarborough Southwest" as the riding name, and since it's not a provincial parliament reference, isProvincial stays false with the fallback riding properly set.

Moving to the next row with the party information, I extract "Liberal" from the link text and set that as the party value. After the loop finishes, since no riding was explicitly captured in the main parsing, the fallback kicks in and assigns "Scarborough Southwest" as the riding. The lookup should return all three values correctly—so why is the test failing? Wait, I'm noticing something in the test HTML structure itself...

The table has both infobox and vcard classes, but my selector table.infobox should still match that since CSS class selectors match any element containing that class. Let me trace through the actual HTML parsing more carefully—maybe there's something subtle about how the whitespace or link structure is being handled in the test data. I'm wondering if the real problem is that the test server isn't properly registering the article page for "/wiki/Doly_Begum" when the category page tries to fetch it. Looking at how the server handles requests, it iterates through the registered paths and sets up handlers for each one, so when fetchDoc makes a GET request to that URL, it should match the registered path and return the HTML body.

Let me check what the fetchDoc function actually does to see if there's something about how it handles the request that I'm missing.

Read shared.go (lines 36-65)
The fetchDoc implementation looks fine. It does a GET and returns the parsed document. Let me add a simple debug test to understand what's happening.

Actually, let me look at the infobox selector issue. We're doing doc.Find("table.infobox tr"). But in the failing test HTML, there's only an HTML body with no actual table class. Let me check my test HTML:


<table class="infobox vcard">
The CSS selector table.infobox should match this since "infobox" is one of the classes. ✓

Now I'm tracing through what happens when we call .Last() on the anchor elements. In that header row, there are two links - one for "Member of Parliament" and one for "Scarborough Southwest" - so .Last() should grab the second one.

That gives us the text "Scarborough Southwest" for the riding, which looks correct. The provincial flag stays false and the fallback riding gets set properly. After the loop finishes, the riding should be assigned from the fallback value, which should also be correct.

I'm not seeing the issue just from reading the code, so I need to add some debugging output to figure out what's actually happening. Let me try writing a more direct test that calls fetchArticle without going through the lookup wrapper to isolate where the problem is.

Actually, looking at the test failure again - the error shows riding="" but we're expecting "Scarborough Southwest", and the test comment says it should fall back to the federal row. The lookup itself succeeds (ok is true), but the riding field comes back empty. So info.party gets set to "Liberal" correctly, but info.riding never gets populated, which means fallbackRiding was never assigned a value in the first place.

That's strange - if we're extracting the text correctly and setting the fallback, why would it be empty? Let me check how the test server is actually serving the category page...

Building the category page handler...

The test server constructs HTML with links to articles, converting wiki paths like /wiki/Doly_Begum into display names. When ensureLoaded parses the category page, it extracts these href attributes and constructs full URLs by combining the base server URL with the path. The handler is registered at the exact same path, so the request should resolve correctly. Let me trace through what HTML the test server actually returns when serving an article to see if there's a mismatch in the response format. I'm realizing the CSS selector should match the table with multiple classes just fine, but I'm wondering if there's something off with how the rows are being processed in the loop—maybe the condition checking for td.Length() is filtering out results unexpectedly. I'm wondering if the issue is how goquery handles <br> tags nested inside <th> elements—maybe it's treating them as block-level and closing the header early, though that shouldn't happen since <br> is inline. Let me try normalizing the text by replacing newlines with spaces instead of using strings.Fields. Actually, the quickest way to debug this would be to add logging to see what th.Text() actually returns and what value fallbackRiding ends up with. So the fallback test doesn't include "(federal)", which means that's not the issue. Looking at the lookup function, since it's returning ok = true, info.party must be non-empty ("Liberal"), but info.riding is coming back empty. That tells me fallbackRiding was either never set in the first place, or it got reset to empty somehow after being assigned. Let me trace through whether the code actually entered that conditional block where fallbackRiding gets assigned.

The assignment happens inside the td.Length() == 0 branch, but I'm wondering if the HTML parser is handling the infobox structure differently than I expect. For the test HTML with the header row containing <th colspan="2">, the td.Find("td").First() call might be finding something unexpected, or the parser might be creating implicit elements I'm not accounting for.

Let me think through the CSS selector too—table.infobox should match <table class="infobox vcard"> correctly in goquery, so that's probably not the issue. I should just write a focused test to see what's actually happening with the parsing. I'm setting up the test with a mock server that serves the HTML for Doly Begum's page, and I'm tracing through how the category path gets processed—the href doesn't contain a colon so it passes the filter, and the article URL gets constructed correctly.

Now I'm walking through the lookup flow: normalizing "Begum" to lowercase, finding the matching entry in the category results, and then fetching the actual article HTML. I need to verify that the infobox table selector will match the table.infobox vcard element in the HTML.

Actually, I'm wondering if Go's HTML parser is modifying the table structure in unexpected ways when it corrects malformed HTML. Or maybe the issue is with how .First() behaves on empty selections—if a row has no <td> elements, row.Find("td").First() would return an empty selection, which would trigger the colspan-2 fallback.

Let me trace through what happens with the actual row containing the party information. When I find the <th scope="row"> element and then grab the first <td>, I should get the link to "Liberal". That means the party field gets set correctly, but the riding field stays empty since it's not being populated in the loop. The fallback riding value from the infobox should kick in instead.

But I'm wondering if there's something odd happening with how the HTML parser handles that first header row with the <br /> tag and multiple links. Let me add some debug output to see what's actually being parsed.

Actually, thinking through how goquery extracts text - it concatenates all the text nodes within that <th> element. So I'd get "Member of Parliament" from the first link, then "for " from the text between the break and the second link, then "Scarborough Southwest" from the final link. The question is whether those pieces are joining correctly or if there's whitespace handling I'm missing.

When .Text() runs, it just concatenates these without adding spaces around the <br> tag - so I end up with "Member of Parliamentfor Scarborough Southwest" all run together. After normalizing to lowercase, that becomes "member of parliamentfor scarborough southwest". Now when I check if this contains " for " with spaces on both sides, it fails because "parliamentfor" has no space between those words. I'm tracing through how goquery parses the DOM structure: the first link's text, then the void <br> element which contributes nothing, then the text node "for " with its leading space, and finally the second link's text. When concatenated together, this gives "Member of Parliamentfor Scarborough Southwest" — the space before "for" in the HTML gets lost because it's positioned right after the <br> tag, and goquery just concatenates the text nodes without preserving that whitespace between elements. I see the issue now—when I actually fetched the real Wikipedia HTML for Doly Begum, there were spaces around the <br /> tag, so the text extraction worked correctly and included "for" as a separate word. The bug only appeared in my test case because I had removed those spaces, causing "for" to concatenate directly with "Parliament" without any whitespace between them. I can see the root cause now—the passing test has proper spacing around the <br /> tag, while the failing test is missing the space before it, causing the text to run together as "Member of Parliamentfor" instead of having " for " as a separate token. I need to either add that space to the test HTML or modify the code to handle this edge case where there's no whitespace around the line break. ated to the previous word without spacing. I could fix this by either correcting the test HTML to match real Wikipedia's format with proper spacing, or making the code more robust by using word boundary matching with a regex pattern like \bfor\b. The simplest approach is probably to insert a space when handling <br> tags in the selector, so the text parsing works correctly regardless of spacing around the tag.

Actually, using \bfor\s would be more robust—it matches "for " with a word boundary before it, so it would correctly identify "for" in "Parliament for" but not in "Parliamentfor". After strings.Fields splits on whitespace and strings.Join recombines, "Parliament\nfor" becomes "Parliament for", which would match the pattern correctly.

Looking at actual Wikipedia HTML for Doly Begum, the structure is <br /> for <a> with spaces around "for", so the test case has incorrect HTML. I should fix both the test HTML to match reality and add word-boundary regex matching as a defensive measure. I see the real Wikipedia HTML includes spaces around the <br /> tag, so I should update the test HTML to match that format and add word-boundary checks to the code for extra robustness. Let me fix the test case for the federal-only scenario by adding the space before <br /> to align with actual Wikipedia markup.

Looking at the existing test that passes, it has </a><br />for without a space, which means the label becomes "member of parliamentfor scarborough southwest" with no " for " substring—so the federal row check correctly skips it since it's looking for that specific spacing pattern.

The failing test only has the federal row and needs to detect " for " in it, but the malformed HTML without the space breaks that detection. The simplest fix is to add a space before <br /> in the test HTML to match actual Wikipedia formatting, and potentially make the detection more robust for edge cases where spacing is inconsistent. I'm simplifying the approach by checking if the label contains "member of" or "member for" instead of relying on the fragile "for" word boundary detection, which would catch all the relevant legislative roles like Member of Parliament, Provincial Parliament, or Legislative Assembly variants.

The issue is in the test HTML — <a>Member of Parliament</a><br />for has no whitespace before <br />, so .Text() returns "Parliamentfor" with no space, and " for " fails to match. But the code logic itself is also fragile. Let me fix both:

Edit wiki_lookup.go
Added 1 line
Let me run the tests to verify the fix works.

Bash Run wiki lookup tests
IN
go test ./internal/scraper/provincial/... -run "TestWikiLookup" -v 2>&1

OUT
=== RUN   TestWikiLookup_BySurname
--- PASS: TestWikiLookup_BySurname (0.02s)
=== RUN   TestWikiLookup_AccentFolding
--- PASS: TestWikiLookup_AccentFolding (0.01s)
=== RUN   TestWikiLookup_ByFullName
--- PASS: TestWikiLookup_ByFullName (0.01s)
=== RUN   TestWikiLookup_Disambiguation
--- PASS: TestWikiLookup_Disambiguation (0.01s)
=== RUN   TestWikiLookup_UnknownName
--- PASS: TestWikiLookup_UnknownName (0.01s)
=== RUN   TestWikiLookup_ArticleCached
--- PASS: TestWikiLookup_ArticleCached (0.01s)
=== RUN   TestWikiLookup_CategoryFetchFailure
--- PASS: TestWikiLookup_CategoryFetchFailure (0.01s)
=== RUN   TestWikiLookup_MultipleCategoriesMerged
--- PASS: TestWikiLookup_MultipleCategoriesMerged (0.01s)
=== RUN   TestWikiLookup_NewProvincialWikiLookup
--- PASS: TestWikiLookup_NewProvincialWikiLookup (0.00s)
=== RUN   TestWikiLookup_PartyQualifierStripped
--- PASS: TestWikiLookup_PartyQualifierStripped (0.01s)
=== RUN   TestWikiLookup_RidingFromColspan2Header
--- PASS: TestWikiLookup_RidingFromColspan2Header (0.01s)
=== RUN   TestWikiLookup_RidingFallsBackToFederalRow
--- PASS: TestWikiLookup_RidingFallsBackToFederalRow (0.01s)
PASS
ok  	github.com/philspins/opendocket/internal/scraper/provincial	0.134s
All 12 tests pass. Running the full suite now.

Bash Run full test suite
IN
go test ./... 2>&1

OUT
ok  	github.com/philspins/opendocket/cmd/crawler	48.910s
?   	github.com/philspins/opendocket/cmd/server	[no test files]
ok  	github.com/philspins/opendocket/internal/auth	0.458s
?   	github.com/philspins/opendocket/internal/clog	[no test files]
ok  	github.com/philspins/opendocket/internal/db	(cached)
?   	github.com/philspins/opendocket/internal/opennorth	[no test files]
ok  	github.com/philspins/opendocket/internal/riding	0.131s
ok  	github.com/philspins/opendocket/internal/scheduler	(cached)
ok  	github.com/philspins/opendocket/internal/scraper	0.838s
ok  	github.com/philspins/opendocket/internal/scraper/provincial	2.252s
ok  	github.com/philspins/opendocket/internal/server	1.106s
ok  	github.com/philspins/opendocket/internal/store	(cached)
ok  	github.com/philspins/opendocket/internal/summarizer	(cached)
ok  	github.com/philspins/opendocket/internal/templates	(cached)
?   	github.com/philspins/opendocket/internal/urlutil	[no test files]
ok  	github.com/philspins/opendocket/internal/utils	(cached)
?   	github.com/philspins/opendocket/internal/visitor	[no test files]
All tests pass. The Wikipedia lookup feature is complete and working:

Party extraction: wikiFirstLinkOrText() returns the first <a> link text, stripping qualifiers like "(since 2026)"
Riding extraction: Two-pass approach — prefers provincial legislature rows (<th colspan="2">Member of ... Legislative/House/National Assembly), falls back to federal MP rows if no provincial row is found
<br/> whitespace: The detection key is "member of" / "member for" rather than " for ", since goquery's .Text() collapses <br/> to nothing rather than a space
All 10 provinces: Covered by provincialWikiCategoryPaths with both 21st- and 20th-century category variants

image.png
631×940
another layer of complexity to consider is that a member can change parties and government levels. see this except from Begum's wikipedia:

Previously, she was the member of Provincial Parliament (MPP) for Scarborough Southwest from 2018 to 2026 and a member of the Ontario New Democratic Party (NDP), serving as deputy leader from 2022 to 2026.

this is reflected in the same table where we scraped that Begum is a "Member of Parliament for Scarborough Southwest":

Deputy leader of the Ontario New Democratic Party
In office
July 13, 2022 – February 3, 2026
Serving with Sol Mamakwa
Leader	
Peter Tabuns (interim)
Marit Stiles
Preceded by	John Vanthof and Sara Singh
Show less
we are going to want to make sure that we register votes for members at the correct time
we are going to want to make sure that we register votes for members at the correct time to line up with when they were in office for a given role
The user is pointing out a complexity with the Wikipedia infobox parsing. Looking at the Begum example:

She was an MPP (Member of Provincial Parliament) for Scarborough Southwest (2018-2026) - this is her provincial role
She is now an MP (Member of Parliament) for Scarborough Southwest (since April 13, 2026) - this is her federal role
She was in the NDP while an MPP, but now is presumably Liberal (since the party field shows "Liberal (since 2026)")
The problem is: when we look up someone for a provincial vote record, we want their provincial party and riding.

The tricky part is that the infobox lists the federal role first since it's most recent, but the party field shows her current affiliation (Liberal), not what she belonged to during her provincial tenure (NDP). So while we can correctly identify the provincial riding from the MPP row, we're pulling the wrong party information if we just use the top-level party field. The core issue is that Wikipedia shows her current party (Liberal as of 2026), but if we're scraping provincial vote records from Ontario between 2018-2026, she'd appear as NDP during that period. The infobox party field doesn't account for temporal changes, so we need to either check the provincial legislature row for historical party info or recognize that the current party may not match the party she held during the votes we're processing. Looking at the actual Wikipedia structure, the infobox uses chronological ordering with the party field typically representing the current or primary affiliation, so I should focus on matching the provincial legislature row with its immediately following party information rather than trying to parse complex date ranges across the entire infobox. For enriching provisional member records from provincial legislatures, the simpler approach is to extract the party associated with that specific office block rather than attempting to correlate multiple party entries with their time periods. The infobox shows her current party as Liberal (since 2026), but for provincial votes she would've been NDP—so we're extracting the wrong party for historical context. I could try detecting the party associated with her provincial role, but that's complex. The more practical approach is accepting this as a limitation: Wikipedia enrichment works well for current members, but for former members who switched parties, we might pull incorrect historical data. I think Option 4 makes the most sense here—if someone has an "Other political affiliations" section, their current party might not match their provincial tenure, so I should just skip the party field and return the riding instead. Looking at the actual Wikipedia structure, I can see the party has a "(since 2026)" qualifier while the other affiliations show their historical party during the relevant period, which confirms this approach would handle the ambiguity cleanly. NDP when she was an MPP, but we're returning her current party affiliation instead. The core issue is that we need to match the party to the specific time period of the provincial vote, not just grab whatever's in the main infobox. Since we don't know the vote year in the lookup function, I'd need to either pass that context through or restructure how we extract party information—ideally pulling it from the provincial legislature role rather than the global party field.

Looking at how Wikipedia actually structures these infoboxes, the party parameter shows the current affiliation while otherparty lists historical ones. For someone who was NDP in the provincial legislature and later became Liberal federally, I should check the "Other political affiliations" section to find the provincial party that matches the relevant time period, rather than defaulting to the current party. I'm examining the Wikipedia infobox structure more carefully—office rows appear in reverse chronological order with the most recent first, so for someone who transitioned from provincial to federal office, the federal role shows up before the provincial one. The party field reflects their current affiliation, while other political affiliations contain historical parties with date ranges. To solve this, I need to match the provincial tenure dates against those affiliation entries to find which party was actually associated with that specific role.

Looking at the current extraction logic, it correctly identifies the provincial riding from the first MPP row, but it's pulling the wrong party since it just grabs the current party field. A more reliable approach would be to skip returning a party altogether when there's ambiguity—if both a Party field and Other political affiliations exist, I'll just return the riding and leave the party blank rather than risk assigning the wrong one.

But actually, I'm realizing the Party field itself tells us something useful: it represents the current party affiliation. If there's a date qualifier like "(since YEAR)" alongside Other political affiliations, that's a signal the person has switched parties or levels of government. In those cases, I should probably skip the party to avoid returning incorrect information.

For most active legislators we'd look up, this edge case probably doesn't matter much—the current code works fine. But when it does come up, the safest approach is to omit the party rather than guess, since the riding is the more reliable piece of information anyway. I should explain this trade-off to the user and ask if they want me to implement that check. with the provincial tenure. If I can't reliably determine the match, I should just skip assigning a party altogether.

Let me think through the implementation: I'd collect the party and any other political affiliations with their date ranges, extract the provincial legislature tenure period from the infobox if it's there, then try to match the party to that timeframe. If the matching doesn't work out, I'll return just the riding without a party rather than guessing wrong.

This is getting complicated, so I should present the problem clearly to the user and propose a concrete solution they can agree with before I start coding it.

The core issue is that parsing dates from Wikipedia text like "June 7, 2018 – February 3, 2026" is messy, and correlating parties with provincial roles through the infobox structure isn't straightforward. The simplest practical approach would be: when I detect a date qualifier in the Party row and there's an "Other political affiliations" section, I should look there for the provincial party instead. But I still need to figure out which affiliation is actually the provincial one by cross-referencing with the provincial legislature row.

Actually, I'm overcomplicating this. Let me step back and find a simpler solution.

I'll just skip returning party information whenever "Other political affiliations" exists—this way I return only the riding. It handles the Begum case correctly (riding=Scarborough Southwest, no party) and doesn't break anything for members with a single party. The tradeoff is losing party data for members who switched parties, but that's safer than returning incorrect information.

This is a real ambiguity. The infobox Party field reflects the current primary party, not the party during provincial tenure. For Begum:

Party row → "Liberal (since 2026)" → we extract "Liberal" ✓ for her current role, ✗ for her provincial record
Other political affiliations row → "Ontario New Democratic (2018–2026)" → the correct provincial party
The provincial MPP tenure row names the correct riding
There are a few options for handling this:

Option A — Skip party on ambiguity: If an "Other political affiliations" row exists, the member changed parties at some point. Just omit the party and return only the riding. Safe but loses party for all multi-party members.

Option B — Date-range matching: Parse the tenure dates from the provincial legislature row ("2018–2026") and match against "Other political affiliations" date ranges. Accurate but fragile — date formats vary across Wikipedia articles.

Option C — Provincial-role-first approach: When we find the provincial legislature colspan-2 row, note its position in the infobox, then look for party affiliations in the rows immediately following it (before the next office header). This relies on the infobox ordering convention — offices are listed reverse-chronologically, and party rows generally appear near the top, not interleaved per office.

My recommendation is Option A — it's correct and safe for the cases that matter. The riding is the more valuable piece of data anyway (it disambiguates same-surname members). Party for historical ex-members who changed party can be populated through other means. Option B sounds appealing but the date parsing is fragile enough to create new bugs.

Want me to implement Option A — detect the "Other political affiliations" row and suppress the party when it's found?

The user is raising a broader architectural concern: when registering votes, we need to make sure we're associating votes with the correct member record that was active at the time of the vote.

This is a more fundamental issue than just the Wikipedia party lookup. Let me think about what this means:

A member like Begum was an NDP MPP from 2018-2026

She then became a Liberal MP from April 2026

If we're processing Ontario provincial votes from, say, 2022, the vote should be attributed to "Begum, NDP MPP for Scarborough Southwest"

If we're processing federal votes from 2026+, it should be attributed to "Begum, Liberal MP for Scarborough Southwest"

The core problem is that our current system probably doesn't track these time-bounded memberships properly, so I need to examine the database schema and member record structures to see what we're actually storing.

Let me understand the current data model before discussing an approach.

Bash Find member record and store definitions
IN
grep -r "MemberRecord\|members table\|CREATE TABLE members\|government_level\|start_date\|end_date\|term" /home/phil/code/opendocket/internal/store/ --include="*.go" -l 2>&1

OUT
/home/phil/code/opendocket/internal/store/store.go
/home/phil/code/opendocket/internal/store/write.go
/home/phil/code/opendocket/internal/store/store_test.go
Bash Find schema/migration files
IN
find /home/phil/code/opendocket -name "*.sql" -o -name "schema*" -o -name "migrate*" | head -20 2>&1

OUT
/home/phil/code/opendocket/node_modules/zod/v4/core/schemas.d.ts
/home/phil/code/opendocket/node_modules/zod/v4/core/schemas.cjs
/home/phil/code/opendocket/node_modules/zod/v4/core/schemas.js
/home/phil/code/opendocket/node_modules/zod/v4/core/schemas.d.cts
/home/phil/code/opendocket/node_modules/zod/v4/classic/schemas.d.ts
/home/phil/code/opendocket/node_modules/zod/v4/classic/schemas.cjs
/home/phil/code/opendocket/node_modules/zod/v4/classic/schemas.js
/home/phil/code/opendocket/node_modules/zod/v4/classic/schemas.d.cts
/home/phil/code/opendocket/node_modules/zod/v4/mini/schemas.d.ts
/home/phil/code/opendocket/node_modules/zod/v4/mini/schemas.cjs
/home/phil/code/opendocket/node_modules/zod/v4/mini/schemas.js
/home/phil/code/opendocket/node_modules/zod/v4/mini/schemas.d.cts
/home/phil/code/opendocket/node_modules/zod/src/v4/mini/schemas.ts
/home/phil/code/opendocket/node_modules/zod/src/v4/core/schemas.ts
/home/phil/code/opendocket/node_modules/zod/src/v4/classic/schemas.ts
/home/phil/code/opendocket/vendor/github.com/pdfcpu/pdfcpu/pkg/pdfcpu/migrate.go
Bash Find SQL files outside vendor/node_modules
IN
find /home/phil/code/opendocket -name "*.sql" -not -path "*/node_modules/*" -not -path "*/vendor/*" 2>&1

OUT
(Bash completed with no output)
Read store.go
Now let me look at the write.go to understand the MemberRecord structure and how members are stored.

Now let me look at the write side and the MemberRecord struct to understand the current schema.

Read write.go
Bash Find MemberRecord usage in bills.go
IN
grep -n "MemberRecord\|CreateTable\|members\|start_date\|end_date\|term_start\|term_end\|active" /home/phil/code/opendocket/internal/scraper/bills.go | head -40 2>&1

OUT
(Bash completed with no output)
Now I have a clear picture of the data model. The key observations:

The members table has a single party, riding, active flag, government_level, etc. — no start_date/end_date or term tracking.
The member_votes table just maps division_id → member_id → vote — no temporal context.
MemberRecord has no TermStart/TermEnd fields.
The user's concern is: a person like Begum can be:

Doly Begum, NDP MPP for Scarborough Southwest (2018-2026) — provincial member record

Doly Begum, Liberal MP for Scarborough Southwest (2026-present) — federal member record

When we have an ON provincial vote from 2022, we need the vote attributed to the provincial Doly Begum (NDP), not the federal one. Currently, if both records exist in the DB, resolveProvincialMemberID would look for members with government_level = 'provincial' for the province — so that should work correctly, since the federal member has a different government_level. But the real issue is what happens if we create a provisional provincial member record for Begum with the wrong party affiliation first.

The core problem is that our schema doesn't handle members across time — a person can change parties, ridings, or government levels between terms, but we're storing a single party and riding per member. Each vote should be linked to the member's actual party and riding at that time, not their current status.

I'm considering whether to create separate records for each term a member serves — so Doly Begum would have distinct entries for her provincial and federal roles with their respective date ranges — or find another way to track this temporal data without fragmenting member identities.

The alternative is adding a member_terms table to capture party, riding, and government level across different periods, potentially linking votes back to the specific term they occurred in. Or I could simply denormalize by storing party and riding directly in the votes table as a snapshot of what was true at vote time.

But realistically, for most provincial scrapers the member data comes from official sources that already have the correct party and riding information, so I might be overcomplicating this before understanding where the actual gaps are.

The real issue is that Wikipedia lookups should only happen for members not already in the database — if someone like Begum is an official provincial MPP, her record should already exist with the correct party affiliation. The question is whether the provincial member data is comprehensive enough to cover historical members or if it only includes current ones, which would determine how often we actually need to fall back to Wikipedia.

The broader architectural problem the user is pointing out is that we're not tracking temporal accuracy — the schema doesn't capture what party a member belonged to at the time they voted, just their current status. This would require tracking member terms and party affiliations across time periods. For Begum specifically, her provincial NDP votes need to stay separate from her current federal Liberal role, but our current setup doesn't enforce that distinction. I think the user is flagging this as a design concern they want to address, so I should confirm my understanding of the problem and propose a concrete minimal approach. I'd recommend adding term dates to the members table so we can match votes to the correct party affiliation based on when they occurred, and for ambiguous cases like party switches, we'd skip the party lookup rather than guess wrong. It's still complex, but presenting it clearly and asking what level of change they want seems like the right move.

Now I'm realizing the core issue: when someone like Begum served in both provincial and federal roles, they'd have two separate member records with different IDs. The provincial votes reference the provincial ID and federal votes reference the federal ID. The real problem surfaces at the join point—when we're processing Ontario provincial votes and looking up "Begum," we need to find her provincial record with the correct party. If she's already in the database from official sources with the right affiliation, we're fine, but if not, the Wikipedia lookup becomes risky because it might pull the wrong party if she switched affiliations between roles.

There are two distinct issues here: first, if we create a provisional record from Wikipedia when she's not in the database, we might assign the wrong party for her provincial votes. Second, the schema itself doesn't handle someone changing parties while staying in the same role—like being an NDP MPP from 2018-2022 then switching to Liberal while still serving as an MPP. The first issue might be solved by not setting a party when there's ambiguity, but the second requires a more fundamental schema redesign. I should focus my response on these two problems and ask which one they want to address first, especially since they mentioned wanting to register votes at the correct time.

Now I'm thinking through how to properly track member terms with time ranges—adding term_start and term_end columns to the members table could help disambiguate which record applies at a given division date. But I'm realizing the current system already uses separate IDs for different roles, so someone who was both an NDP MPP and a Liberal MP would have two records. The real question is whether the ID generation and resolution functions handle this correctly for provincial members.

Let me look at how provisional member records are created in the crawl to understand the full picture.

Read crawl.go
i prefer option b: date range matching
Now I have a clear picture. Let me think about the temporal problem.

Current state:

members table: one row per person in a given role. No start_date/end_date. Has active bool.
member_votes table: maps (division_id, member_id) → vote. No time context beyond what the division's date carries.
Provisional member IDs: e.g. on-historical-doly-begum
The temporal problem with Begum:

For ON provincial votes (2018-2026):

Vote source says "Begum" voted Yea on some bill
resolveProvincialMemberIDFromCandidates looks for an ON provincial member named Begum
If she's in the official ON member data (from Represent API), she'd have an ID like on-doly-begum-1 with party=NDP, government_level=provincial
If she's NOT in the DB, we create provisional record on-historical-doly-begum with party=Liberal (wrong) from Wikipedia
For federal votes (2026+):

She would be a federal member with a separate ID (not scraped by provincial scraper)
So the immediate issue is that we're conflating provincial and federal roles, and when we can't find her in the provincial data, we're falling back to a provisional record with incorrect party information.

The deeper issue is that our schema can't handle party changes during a member's tenure — if Begum was NDP from 2018-2022 and switched to Liberal before 2026, votes from those different periods should reflect her party at the time, but we only store one party value per member. The user wants to ensure votes are properly attributed to members across their full political history, which means we need a way to track party affiliation over time.

I'm thinking the solution is adding term dates to the members table — term_start and term_end fields that let us match votes to the correct member record based on when they were actually in office. When resolving a member ID, we'd filter candidates using the vote date to ensure we pick the right person with the right party for that time period.

The alternative is keeping separate records for each role, which is what we're already doing, but then the provincial scraper only looks at provincial records and can't connect them to federal ones.

The real problem surfaces with provisional members not in official data or members who switched parties within the same role. A third option would be to snapshot party and riding directly in the votes table itself, recording what party they belonged to at the time of voting—this is most accurate but requires knowing the party affiliation at that specific moment, which circles back to the same dependency issues.

For now, the practical fix is straightforward: if Begum's already in the official provincial member list as NDP, the Wikipedia lookup never triggers for her votes. The only gap is when she's missing from official data and we're forced to create a provisional record with potentially wrong party information. I should present these options to the user and see how much effort they want to invest in a long-term solution versus a quick patch. A (suppress party when ambiguous).

The bigger design question: do we need term tracking? Looking at the current crawl architecture:

We scrape the CURRENT legislature/session
Historical votes are only scraped if there's fallback logic (previous session)
So most of the votes we're processing are for CURRENT members
By definition, current members haven't changed parties yet (or if they have, the official data would reflect the current party)
The resolveProvincialMemberIDFromCandidates already filters by province and government_level=provincial, so federal members don't interfere with provincial vote matching.

Where it gets tricky:

Historical votes: votes from a previous session when a member had a different party
Cross-government-level: same person as provincial AND federal member
For now, I think the most pragmatic answer is:

Fix the Wikipedia party issue (suppress ambiguous parties)
A proper term tracking system would need a schema migration, but that's worth doing down the line
In the meantime, I can add term_start and term_end fields to the schema and populate them as we're able to
★ Insight ─────────────────────────────────────

The root tension here: the members table is a snapshot (one row = current state), but vote history is temporal. A member's party/riding at vote time is what matters, but the schema has no way to express "was NDP from 2018-2026, then Liberal from 2026+." The current design separates provincial and federal records by government_level, which handles the role change, but not party changes within a role.
─────────────────────────────────────────────────

Here's how I read the problem. Two distinct cases:

Case 1 — Role change (Begum's actual situation): She's on-doly-begum-1 (provincial, NDP) AND eventually a federal member under a different ID. The provincial vote scraper filters by government_level='provincial', so federal Begum never contaminates provincial votes. This case is already handled by the separate-records model — as long as the official provincial member record exists with the correct party.

Case 2 — Party change within a role: A member who was NDP MPP for all of 2018–2022, then crossed the floor to Liberal while still serving as MPP. Votes cast in each period should reflect the correct party. The current schema has no way to represent this — one member record, one party.

Case 3 — Provisional record timing: The Wikipedia lookup fires when someone is NOT yet in the official member data. Wikipedia shows their current party, which may differ from their party at the time of the provincial votes.

The minimal schema change that addresses all three is adding term_start and term_end to members:


ALTER TABLE members ADD COLUMN term_start TEXT;   -- ISO date, nullable
ALTER TABLE members ADD COLUMN term_end   TEXT;   -- NULL = currently serving
With this, resolveProvincialMemberIDFromCandidates can accept a vote date and filter: term_end IS NULL OR term_end >= vote_date. For someone who changed parties, you'd have two separate rows (different IDs) for the two terms.

For the Wikipedia issue specifically, the safest short-term fix is still Option A (suppress party when "Other political affiliations" exists), since an incorrect party on a provisional record would then be sticky in the DB.

Before designing the migration, two questions worth clarifying: do you want to tackle term tracking now as a schema migration, or is the immediate priority just fixing the Wikipedia party correctness? And do you have term date data available from the sources we're already scraping, or would this require a new data source?

yes tackle term tracking now and update the database table to be temporal
The user wants Option B: date-range matching for party resolution from Wikipedia. Let me think about what this means in the context of fetchArticle.

The idea is:

When we find a provincial legislature row (the colspan-2 "Member of Provincial Parliament for [Riding]"), we also look for the tenure dates associated with that role (e.g., "June 7, 2018 – February 3, 2026")

When we find the "Party" row with a date qualifier like "(since 2026)", and we have the "Other political affiliations" row with "Ontario New Democratic (2018–2026)", we can match the party to the provincial tenure period.

Looking at the actual Wikipedia infobox structure for Begum, the office header and its associated dates come first, followed by the party information at the top level — so I need to track the tenure dates from the office block and use them to resolve which party affiliation corresponds to the provincial role. Looking at the actual HTML structure, I can see the party information is listed separately from the office tenure dates. So I need to extract the provincial tenure range from the "In office" row under the provincial legislature header, then find which party affiliation overlaps with that date range to determine what party she belonged to during that provincial service. Now I need to handle the parsing logic for these varied date formats from Wikipedia infoboxes. The core insight is that I only need to extract years for comparison purposes, so I can use regex patterns to identify "since YEAR", "YEAR1–YEAR2" ranges, and full dates where I just pull out the years. This should cover most of the formatting variations I'll encounter.

For the actual implementation, when I'm processing the infobox HTML, I'll track which provincial legislature section I'm in by watching for colspan-2 header rows. Once I identify a provincial office block, I'll scan the following rows for "In office" entries and extract their year ranges. Simultaneously, I'll collect all party affiliations with their associated years from the "Party" and "Other political affiliations" rows. After processing the entire infobox, I can match the provincial tenure years against the party year ranges to find the best overlap and determine which party was active during that provincial service. Defining the state machine logic...

For parsing dates in office and party affiliations, I need to extract year ranges from various formats—things like "June 7, 2018 – February 3, 2026" becomes [2018, 2026], while "Incumbent" or open-ended tenures get marked as [year, 0]. Same approach for party strings like "Liberal (since 2026)" or "Ontario New Democratic (2018–2026)". Then I can test overlap between provincial tenure and party affiliation ranges to find the best match based on longest overlapping period.

Looking at the actual Wikipedia HTML structure for {{Infobox officeholder}}, the "In office" dates appear in dedicated rows within the infobox, typically showing the date range in a single cell. The party information comes separately, not nested inside the office section. I'm realizing the infobox structure has two main sections: office rows followed by personal info rows like party affiliation. I can parse this in a single pass by tracking the current office block context, then when I encounter party or affiliation rows, I'll associate them with the appropriate timeframes. Once I've gone through all rows, I'll consolidate the data into the final structure.

Now I'm examining how the "In office" dates are actually laid out in the HTML—they might be in a dedicated row or embedded in the office header itself, so I need to handle both cases. I can see the full picture now—Begum's party affiliation changed from Ontario NDP to Liberal in 2026, which aligns with her federal MP role. I need to design data structures to track office positions with their tenure dates and then match provincial service years against party affiliations to determine when she switched parties.

Defining the Go types... I'm setting up the parsing logic to track office tenures and party affiliations, storing whether each office is provincial or federal, along with start and end years. As I process each row, I check for office headers to initialize a new tenure record, then capture the date ranges from "in office" rows. Once I hit the party section, I reset the office context and parse the main party affiliation plus any other political associations, extracting names and their corresponding year ranges. find all 4-digit years works for most cases. For party affiliations, I need to extract both the party name and the date range—"since 2026" gives me a start year with no end, while "2018–2026" gives me both bounds. When matching parties to provincial tenure, I'll compute the overlap between each party's dates and the provincial service period, with the party showing the most overlap winning the match. I need to make sure I'm looking at the right office block—when calculating overlap for provincial tenure, I should use the dates from the "Member of Provincial Parliament" section (2018-2026), not the federal MP dates. For current members marked as "Incumbent," the "In office" row might just show a start date or "present," so I need to handle cases where the end date is missing or unclear.

Now I'm implementing the revised fetchArticle function... Parsing the infobox table rows...

Now I'm handling the "other political affiliations" section by iterating through each link and collecting party names, though extracting precise date ranges per link is tricky with goquery so I'll parse dates from the full cell text instead. Then I'm checking for constituency or electoral district fields, and for provincial tenure dates when in a provincial block, extracting the start and end years from the cell text.

Continuing with the office header parsing...

Now I'm matching the party affiliation to the provincial tenure by finding which party has the most overlap with the tenure dates, iterating through the main party and any alternatives to calculate the year overlap. Parsing the "Other political affiliations" cell is tricky since it can contain multiple parties with their own date ranges, but for most cases like Begum's page there's just one entry. I can extract the party name from the link text and then use regex to pull out the year range from the full cell content. I need to handle the case where a cell has multiple party affiliations separated by line breaks. The approach is to extract the raw HTML, normalize the line breaks, then parse each line to pull out the party name and associated dates.

Writing the parsing logic... use if there are multiple affiliations. Let me just implement the core logic and handle the straightforward case first, with a fallback for edge cases.

My approach: parse the "Party" cell to extract the party name and any "(since YEAR)" qualifier, then parse "Other political affiliations" to get a list of past parties with their year ranges. If I have provincial tenure information, I'll compare all parties against that timeframe and pick whichever has the most overlap. Otherwise, I'll default to the current party.

For extracting years from text segments, I need a helper function that finds year patterns...

Writing the year parsing logic... I'm finishing the overlap calculation logic that handles present-day affiliations by using a sentinel value, then testing it against the Begum example to verify the NDP affiliation correctly wins with 8 years of overlap. Now I need to parse the "Other political affiliations" cell to extract additional party memberships.

For cells with multiple parties, I'll extract the party name from each link and then pull the associated date text that follows it until the next link appears. If I can't reliably parse dates for some parties in a multi-affiliation scenario, those parties default to zero overlap, which naturally favors the party with the clearest date range. if the main party has no overlap (e.g., starts after provincial tenure ends), any other party with unknown dates would also have 0 overlap. That's not quite right.

Let me think about the fallback more carefully. If I can't parse dates for a party, I should assume it's valid across the entire known history—treating missing dates as spanning from year 0 to present. For overlap calculation, a party with no dates would naturally overlap with the provincial tenure since its range is essentially unbounded. But if the main party has a "(since YEAR)" and that year is after the provincial tenure ends, I know it doesn't apply.

This is getting complicated, so let me simplify the approach. I'll extract the main party name and its start year from patterns like "(since YEAR)", then look at other party affiliations and their year ranges. When I have a provincial tenure year range, I can filter out parties with no overlap and pick the one with the most overlap, falling back to the main party if nothing remains. If the tenure range is unknown, I just use the main party. For parsing, the main party gets the "(since YEAR)" pattern, other affiliations get "(YEAR1–YEAR2)" or similar, and anything without dates gets treated as always valid. In the Begum example, the main party "Liberal" starts in 2026, while "Ontario New Democratic" spans 2018–2026. The current rewritten thinking ends with code structure, so I'll continue with a short phrase since the next thinking is describing more implementation details about the same parsing logic.

Still parsing the data structure... Writing the text extraction logic... Parsing each segment to extract the link and associated dates...

I'm considering whether to parse each part as mini-HTML to isolate the link and extract dates from the surrounding text, though that approach feels a bit heavy. Instead, I'll go with something simpler: if there's a single link in the cell, I'll pull the dates from the full cell text, but if there are multiple links, I'll need a different strategy.

Actually, the "Other political affiliations" section typically renders each party on its own line using <br> tags, so splitting by those tags should handle the common cases cleanly. Let me write the implementation now and keep it straightforward, handling the typical scenarios while being clear about what it can and can't do.

I'm planning to add a helper function that uses regex to extract 4-digit years from text, handling cases like "since 2026" (returning just the start year) or "2018–2026" (returning both), then build out the parsing logic from there. Still writing the parsing logic...

Now I'm realizing the test case might already pass as-is since without provincial tenure dates, the fallback to the main party "Liberal" matches what the test expects, so I need to verify this logic holds before making changes. I'm looking at how the test helper function constructs the HTML with party, qualifier, and riding information, including an "Other political affiliations" row that shows historical party data. The existing test expects the Liberal party to be returned when the qualifier is "(since 2026)", but with the new algorithm that checks for provincial roles with date ranges, I need to verify whether it should still pick Liberal or switch to the historical NDP affiliation based on the "In office" dates.

Since the test HTML is missing the "In office" row entirely, the algorithm falls back to the main party and returns Liberal as expected. However, this doesn't actually test the new date-matching behavior I'm implementing. I need to create a separate test case with complete HTML that includes the "In office" dates so I can verify the algorithm correctly identifies and returns the NDP affiliation when the date ranges align.

Building the new test HTML... I'm checking whether the test expectations still hold with the new algorithm—since there's no "In office" date range for provincial tenure in this case, it falls back to the main party affiliation of "Liberal", which matches what the test expects. is to keep memberArticleHTMLWithQualifier as a base helper and add an optional parameter for "In office" dates, so I can test both the qualifier-stripping behavior and the new date-range logic without duplicating the HTML fixture. just need to add a new test case for the date-range matching scenario. I'm reconsidering whether memberArticleHTMLWithQualifier should be updated — it deliberately mimics a real Wikipedia page for someone who changed parties, which would naturally have "In office" dates, so the current test without those dates is somewhat artificial. But since the test is specifically about stripping the qualifier, I can keep it as-is and treat it as testing the fallback behavior. Now I'm thinking through how to track the inProvincialBlock state across loop iterations — I need to capture a pointer to a boolean so it persists as the closure iterates through the table rows.

For detecting the "In office" row, I need to check that we have a valid th/td pair, the label contains either "in office" or "assumed office", we're currently in a provincial block, and we haven't already captured the provincial tenure dates.

When I find a match, I'll parse the year range from the cell and populate the start and end years. The key insight is that inProvincialBlock needs to stay true while processing these sub-rows — it gets set to true when we hit a provincial header and remains true until we encounter the next office header. I need to figure out when inProvincialBlock should reset to false—it should happen when we encounter the next colspan-2 header row for a different office, not when we see sub-rows like "In office" or "Preceded by". The key is that in the current code, when we process "In office" rows, we need to check if we're still within the same provincial block before deciding how to handle the data. I'm realizing there's a complication with multiple provincial office sections—if someone held provincial office in different ridings at different times, I only want to capture the first one since that's most relevant for vote matching. Since I'm already checking provincialTenure == nil before setting it, I'd naturally get the first match, but I need to make sure inProvincialBlock stays false after the first provincial block ends so subsequent provincial headers don't keep triggering the parsing logic.

Actually, thinking about this more carefully: Wikipedia lists offices in reverse chronological order, so the first provincial office I encounter is the most recent one. That's probably what I want anyway—if someone served as MPP twice, the recent tenure is more likely to overlap with the votes I'm matching. So I should just capture the first provincial tenure I find and stop looking.

Let me stop overthinking and write the code now. Writing the parsing logic... For "Assumed office", the date is just a start date with no end component. Now I'm writing a regex-based function to extract year ranges from text—it finds all four-digit years matching the 1800s or 2000s pattern, then returns the first as the start year and the last as the end year if multiple matches exist.

Still writing overlap logic... I'm noticing a potential issue with how undated parties are handled—when a party has no date information (startYear=0), it calculates an overlap as if it spans from year 0 to the current range, which could incorrectly prioritize it over a properly dated party. For the actual use cases here, this shouldn't be a problem since either there's only the main party with no alternatives, or there's a dated party change to compare against.

But if the main party is undated and other parties have specific date ranges, I should assume the main party is the current affiliation that started after the others ended, so I need a tiebreaker to handle that scenario correctly.

The cleaner approach is to only perform date-range matching when the main party has a known start year—otherwise just return the main party name directly. This sidesteps the ambiguity and handles both current members with no party changes and those with documented transitions.

For extracting the "(since YEAR)" pattern, I'm parsing the party cell text to pull out the start year. A cell like "Liberal (since 2026)" yields start=2026, while a plain "NDP" with no date info stays at start=0.

Parsing the party cell text... Parsing the cell HTML by splitting on <br> tags and then extracting the link text and associated years from each segment...

Actually, I'm overcomplicating this. I should just work with the plain text content of the cell and extract all the years I can find, rather than trying to parse the HTML structure. I need to verify how goquery actually handles <br> tags when extracting text. Looking at the test cases, a single affiliation like "Ontario New Democratic (2018–2026)" comes through cleanly, but for multiple affiliations separated by <br> tags, the text concatenation might not preserve the separation I need. Let me trace through what actually happens when there are multiple parties in one cell.

The challenge is that without explicit separators between the concatenated text, I can't easily tell where one party's information ends and another begins. I'm going to focus on handling the straightforward case of a single "Other political affiliations" entry first, then tackle multiple entries by trying to match the text portions that follow each link back to the party names.

Writing the parsing logic... I'm testing the logic with a few examples to verify it handles the year extraction correctly. For the single party case, it works as expected, but when I have multiple parties concatenated together, the truncation to 25 characters might cut off the second year, causing the parser to grab the wrong values. I need to reconsider whether this approach reliably captures both years in the range.

The real problem is that when multiple parties are listed back-to-back, the text after the first link contains years from subsequent parties too. I should instead focus on extracting years only from the text immediately following each link, before hitting the next one or a separator.

Refining the parsing logic...

I need to account for the fact that the en-dash character takes up multiple bytes in UTF-8, so a year range like "(2018–2026)" is longer than it appears. I'll bump the character limit to 20 to safely capture the full pattern, then test it against a case like "Party1 (2018–2022)Party2 (2012–2016)" to make sure each party gets its own years. Handling Unicode string slicing...

Actually, the real concern is whether we'll accidentally grab years from the next party if they're too close together. Most of the time there's enough separation, so let me simplify this by using a regex that captures just the first year range in parentheses after the party name, rather than trying to truncate the text beforehand.

Writing regex patterns... I need to handle both parenthesized year ranges like "(YYYY–YYYY)" and unparenthesized date ranges like "June 7, 2018 – February 3, 2026". Let me create a more flexible parsing function that can extract years from either format, checking for parenthesized patterns first, then falling back to finding any four-digit years in the text.

Writing the year parsing logic... I'm verifying the year parsing logic works correctly across different text formats—dates with ranges, "since" patterns, and single years all extract properly. The function handles the multiple-affiliation case well since it only parses the text after the link name. I think this implementation is solid enough to move forward with.

Now I'm writing the fetchArticle function that retrieves and caches Wikipedia articles, then parses the infobox to extract party affiliations and tenure information... Parsing political affiliations and extracting party names with their service years from the text, then checking for constituency and office tenure information... Parsing the riding information and determining whether it's a provincial or federal position based on keywords in the header label, then setting up the tenure tracking and fallback riding accordingly. Next I'm handling party selection by matching against date ranges when available, starting with the main party and checking against other parties if tenure dates exist. I'm noticing a logic issue with how inProvincialBlock gets set based on whether provTenureStart is zero. The first time we encounter a provincial header, provTenureStart is still 0, so we set inProvincialBlock = true. But if that first header doesn't have an "In office" row—like for an incumbent MPP—then provTenureStart stays 0, and we'd incorrectly treat the second provincial header as if we're still in the first block. Though actually, if we can't find tenure dates, the date matching won't work anyway, so this might not be a real problem.

For an incumbent MPP, the infobox structure is different—it shows "Incumbent" as a full-width cell rather than a label-value pair, followed by an "Assumed office" row with the start date. Our code already handles this case by checking for both "in office" and "assumed office" labels.

When parsing "June 7, 2018" from the assumed office date, we extract just the year 2018 as the start of their provincial tenure, with no end date since they're still serving. For an incumbent who hasn't switched parties, we use their main party affiliation directly. But if they've recently changed parties—say from NDP to Liberal in 2026—we'd have the Liberal party as their current affiliation with a start year of 2026, while the NDP tenure from 2018 to 2026 goes into the other parties list.

The overlap calculation is causing issues though. When I compute the year overlap between the Liberal period [2026, ongoing] and the full provincial tenure [2018, ongoing], I get 7973 years of overlap, which vastly outweighs the NDP's 8 years. This is backwards—for a current MPP who switched from NDP to Liberal, we should be showing their current party as Liberal, not picking based on overlap duration. The problem is that the ongoing tenure (end=0) is being treated as extending to year 9999 in the calculation, which inflates the overlap for whichever party has the later start date.

Actually, thinking about this more carefully—if someone is currently an active MPP, they should already exist in the official provincial member database, so the Wikipedia lookup wouldn't even run for them. The Wikipedia fallback is only for historical members who've left office (where provTenureEnd > 0) or very recent additions not yet in the system. For historical members, the tenure has a defined end date, so the algorithm should work correctly. The edge case of someone switching parties while still in office probably doesn't come up often enough to matter for the Wikipedia lookup.

But Begum is a specific example worth tracing through: she was an NDP MPP from 2018 until February 2026, then became a Liberal MP in April 2026. The voting records we're processing are from her NDP tenure. When calculating the overlap between her main party (Liberal, starting 2026) and her provincial tenure (2018-2026), the math shows zero overlap since the Liberal tenure starts after the provincial tenure ends. But her NDP party record (2018-2026) should match perfectly with the provincial tenure window.

Now I'm reconsidering whether to update the existing test or leave it unchanged. The test currently uses HTML with a Liberal qualifier and NDP affiliation dates, but no "In office" dates specified. With the new algorithm, I need to check how the provincial tenure calculation would work in this scenario.

Since there are no "In office" dates in the HTML, the provTenureStart would be zero, which means the algorithm falls back to using the main party name "Liberal" — so the test still passes, just for a different reason than originally intended. Rather than modifying the existing test, I should create a new test case that specifically validates the date-range matching behavior when full "In office" dates are present in the HTML.

Writing test HTML with infobox structure... I need to organize the helper functions parseWikiYearRange and yearOverlap at the package level in wiki_lookup.go, then update fetchArticle to use the new date-range matching logic. I also need to add a test case that verifies the party lookup works correctly when filtering by date overlap, while ensuring the existing qualifier-stripping test still passes.

Now I'm reconsidering the label matching for "political party" — specifically whether resetting inProvincialBlock when we encounter that label could cause issues if the party row appears before office rows. But looking at typical Wikipedia infobox structure, party information comes after all the office entries, so it's safe to reset the flag there without losing any office context. I'm confirming the structure differences between past and current officeholders—past ones use "In office" with a full date range, while incumbents use "Assumed office" with just a start date, and our regex handles both cases. Now I need to implement the actual parsing code, and I'm thinking through the yearOverlap function to handle edge cases like when end dates are zero (meaning currently active or unknown) versus when one or both dates are defined. max(2026, 2018)=2026 - 2026 = 0`.

So testing this against the provincial tenure 2018-2026: "NDP" with no dates gets an overlap of 8, "Liberal since 2026" gets 0, so NDP correctly wins. This approach handles the edge case properly—when a party has no dates, we can't exclude it, but we can exclude parties with dates that don't actually overlap. And if multiple parties exist for the same province, one with no dates will beat one with a narrower date range, which is the right behavior since we're treating undated parties as spanning the entire period.

The practical reality is that the main party field is current (and only gets a date qualifier like "since 2026" if there was a recent change), while other affiliations are historical and already have year ranges. So if the main party has no qualifier, it means they've always held that affiliation and there won't be other rows to compare against anyway.

But I'm second-guessing the logic: should I attempt date matching even when the main party has no start year but other parties do exist? That would be an edge case where the main party's dates are unknown but the historical affiliations have specific ranges—which seems like a data inconsistency that probably shouldn't happen.

Let me simplify: I'll use provTenureStart > 0 && len(otherParties) > 0 as the gate for date matching. This ensures we only do overlap calculations when we have both tenure dates and alternative parties to compare against. For the case where the main party has no start year but other parties exist, I'll just skip the date matching logic entirely. I'm verifying the label matching logic for both test and real data—"party" matches directly, while "political party" is caught by the contains check. Now I need to handle the colspan-2 rows that don't contain "member of", like position titles, and figure out how they affect the provincial block tracking.

Looking at the current code, when we hit a colspan-2 row without "member of" or "member for", we return early without resetting the inProvincialBlock flag. This means if we were previously in a provincial section and then encounter something like "Deputy leader", the flag stays true when it should be cleared. I need to set inProvincialBlock = false before returning in that case.

Actually, thinking through the sequence more carefully: we capture provincial tenure dates only once when provTenureStart is still zero. So even if inProvincialBlock incorrectly remains true after the "Deputy leader" row, we won't double-capture dates since we've already set provTenureStart from the actual provincial office dates. But it's still a logic bug that should be fixed for correctness. deputy leader role wouldn't be captured because provTenureStart is already set from step 2, so the condition to capture dates fails. The key insight is that the provincial tenure dates get captured at the right time, and subsequent office headers that don't match the "member of" pattern don't interfere with that.

To be safer though, I should explicitly reset inProvincialBlock whenever we hit a colspan-2 header that isn't a "member of" row—this prevents any edge cases where the order of rows might change. I'm also considering what happens if the "In office" row for the provincial block somehow appears after the deputy leader header instead of before it, which would break the current logic. I'm tracing through the algorithm step by step: when I encounter "Member of Parliament" I set the provincial flag to false, then when I hit "Member of Provincial Parliament" I flip it back to true and capture the tenure dates from June 2018 to February 2026. The algorithm correctly skips over the deputy leader role since that's not a "member of" position, and ignores the succession information.

In post-processing, I'm comparing the party affiliations against the provincial tenure window—the Liberal party only appears in 2026 which doesn't overlap with the 2018-2026 provincial service, while the NDP affiliation spans the entire period, so NDP gets selected as the primary party for that role.

I'm also verifying that the inProvincialBlock flag correctly prevents the federal MP's "assumed office" date from being misinterpreted as provincial tenure, which it does.

Now I need to implement this logic, and I'm thinking through how to handle the parseWikiYearRange function, particularly dealing with en-dashes in UTF-8 encoding when extracting years from date ranges.

Writing the year parsing function... Writing helper functions...

Now I'm thinking through the tie-breaking logic for when multiple parties have the same overlap duration with the target year range. I need to decide how to handle that edge case, though in practice it seems unlikely given the specific year ranges involved.

I'm going to write out the updated code for wiki_lookup.go with the necessary imports and the overlap calculation logic, then update the test suite to cover both the existing case where no "In office" dates are present and a new scenario that matches the Begum example with proper date ranges. I need to handle the label matching generously to catch variations like "in office" or "assumed office", and I'm confident the logic correctly skips federal MP dates since the provincial block flag is false at that point. Now I'm ready to implement the actual parsing logic, though I need to figure out how to extract the party name from cells that include text like "Liberal (since 2026)" — I should use the wiki link extraction to get just the party name itself.

Still writing parser...

Actually, I'm noticing a potential issue with the order of operations in fetchArticle — I'm assigning info.party after the document parsing loop completes, but I should double-check whether that's happening at the right point in the code.

Writing the fetchArticle function... Parsing political affiliations from the infobox...

Now I'm handling the "in office" or "assumed office" labels to capture provincial tenure dates when we're in a provincial block, then checking for full-width office headers that indicate a "member of" position and extracting the riding information from the last link in that header.

Extracting the riding name and determining whether it's a provincial or federal position based on keywords like "legislative assembly" or "provincial parliament", then setting the riding and tracking whether we're still in a provincial block accordingly.

Now I'm moving into the party selection logic, preparing to match parties based on date ranges when we have provincial tenure dates and multiple party options to choose from. I need to add the regex variables and helper functions at the package level. Looking at the structure, I'll place them after the existing constants. But I'm noticing a potential issue with how I'm parsing party affiliations when multiple parties appear in the same cell—using strings.Index to find the party name could match the wrong occurrence if similar party names are present, so I need to refine that logic. I need to parse the HTML structure more carefully instead of relying on string searching. Looking at how these affiliations are actually laid out in the HTML, they're either single entries with a link and year range, or multiple entries separated by <br> tags, each with their own link and dates. Let me examine the actual structure to extract each affiliation block properly. I'm working through how to parse the td content by iterating through child nodes and checking whether each one is a link element or text node, then extracting the party names and dates accordingly. The goquery library wraps the underlying html.Node type, so I can check node types directly. I think the simplest approach is to get the inner HTML, split it by common delimiters like <br> and <li> tags, then extract links and dates from each segment.

Parsing each HTML segment...