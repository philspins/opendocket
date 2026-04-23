# Calendar crawl errors

## Nova Scotia
crawler error...
2026/04/22 17:07:45 [calendar] legislature schedule crawl warning: Get "https://nslegislature.ca/legislative-business": EOF
Use https://nslegislature.ca/get-involved/calendar instead. "Month" tab shows calendar for current month. Any day where the date is a hyperlink to another page is considered a session day.

## New Brunswick
Incorrectly parsed today (4/22/26) as on break. Check that crawler is correctly using https://www.legnb.ca/en/calendar
Green highlighted days are in session

## Newfoundland
Incorrectly parsed. Make sure we are using https://www.assembly.nl.ca/HouseBusiness/ParliamentaryCalendar.aspx
https://www.assembly.nl.ca/pdfs/ParliamentaryCalendar2026.pdf
Green highlighted dates are in session

## Quebec
Incorrect. Update to use https://www.assnat.qc.ca/en/document/211091.html
Session days are highlighted in blue, green and maroon

## Ontario
Correct!

## Manitoba
Incorrect. Update crawler to parse https://www.gov.mb.ca/legislature/business/sessional_calendar.pdf
Grey highlighted days are in session

## Saskatchewan
Incorrect. Update to use https://www.legassembly.sk.ca/media/bdcjdifz/30l3s-calendar.pdf
Grey highlighted days are in session

## Alberta
Correct!

## British Colombia
Correct!