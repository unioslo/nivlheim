function browseHost(certfp) {
	APIcall("mockapi/browsehost.json", "browsehost", "#placeholder_browse");
}

function browseFile(fileId) {
	// the pushState code doesn't work yet -- needs debugging
	//window.history.pushState(null, null, "/browse.html?f="+fileId);
	APIcall(
		//"mockapi/browsefile.json",
		"http://127.0.0.1:4040/api/v0/file?fields=lastModified,hostname,filename,"+
		"content,certfp,versions&fileId="+fileId,
		"browsefile", "#placeholder_browse")
	.done(function(){
		$("select#selectVersion").val(fileId);
		$("select#selectVersion").change(function(){
			browseFile($(this).val());
		})
	});
}

function readyFunc() {
	var p = getUrlParams();
	if (p['c']) {
		browseHost(p['c']);
	} else if (p['f']) {
		browseFile(p['f']);
	}
}

$(document).ready(readyFunc);

/* this code doesn't work yet -- needs debugging
window.onpopstate = function(event) {
	console.log("popstate");
	readyFunc();
}
*/
