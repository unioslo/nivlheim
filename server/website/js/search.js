var spinnerhtml;

$(document).ready(function(){
	var p = getUrlParams();
	$('input#search').val(p['q']);
	spinnerhtml = $("#placeholder_searchresult").html();
	APIcall("mockapi/search.json", "search", "#placeholder_searchresult");
});

function newsearch() {
	// put the spinner back
	$('#placeholder_searchresult').html(spinnerhtml);
	// perform the new search
	var q = $('input#search').val();
	APIcall("mockapi/search.json", "search", "#placeholder_searchresult");
}
