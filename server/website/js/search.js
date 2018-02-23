var spinnerhtml, query = "";

$(document).ready(function(){
	// copy the search string from the url parameter to the search input field
	var p = getUrlParams();
	$('input#search').val(p['q']);
	// make a copy of the spinner html
	spinnerhtml = $("#placeholder_searchresult").html();
	// add event listener for back button
	window.addEventListener('popstate', popstate);
	// search
	query = p['q'];
	performSearch(query);
});

// This function is called when the user clicks "search" or presses enter
function newSearch() {
	// put the spinner back
	$('#placeholder_searchresult').html(spinnerhtml);
	// fake the url
	query = $('input#search').val();
	history.pushState({"query":query}, null, "/search.html?q="+query);
	// perform the new search
	performSearch(query);
}

function performSearch(q) {
	APIcall(
		//"mockapi/searchpage.json",
		"/api/v0/searchpage?q="+encodeURIComponent(q)+
		"&page=1&hitsPerPage=10&excerpt=80",
		"search", "#placeholder_searchresult");
}

function popstate(e) {
	if (e.state) {
		query = e.state.query;
	} else {
		var p = getUrlParams();
		query = p['q'];
	}
	$('input#search').val(query);
	$('#placeholder_searchresult').html(spinnerhtml);
	performSearch(query);
}
