$(document).ready(function(){
	Handlebars.registerHelper('formatDateTime', function(s){
		if (!s) return "";
		var t = moment(s);
		return t.fromNow() + ' (' + t.format('D MMM Y HH:mm') + ')';
	});
	Handlebars.registerHelper('urlescape', function(s){
		if (!s) return "";
		return encodeURIComponent(s);
	});

	var routes = {
		'/browsehost/:certfp': browseHostByCert,
		'/browsefile/:fileId': browseFileById,
		'/browsefile/:hostname/:filename': browseFileByName,
		'/search/:query': searchPage,
		'/search': searchPage,
		'/': showFrontPage
	};

	var router = new tarantino.Router(routes);

	router.configure({
		html5history: false,
		notfound: function(){
			$("div#pageContent").html('<section class="section">'
				+'<i class="fas fa-question fa-2x"></i>'
				+'<i class="fas fa-exclamation fa-2x"></i> '
				+'Oops. Something went wrong...</section>');
		}
	});

	router.param('fileId', /(\\d+)/);
	router.param('certfp', /([0-9A-F]{40})/);
	router.param('hostname', /([\\w\\.]+\\w+)/);
	router.param('filename', /([A-Za-z0-9_\\.~\\-]+)/);
	router.param('query', /(.+)/);

	router.init('/');

	$.get('/version.txt', function(data){
		$("span#navbarVersion").text('Version ' + data);
	});
});

function showFrontPage() {
	renderTemplate("frontpage", {}, "div#pageContent")
	.done(function(){
		$("button#searchButton").click(newSearch);
		$("input#search").keyup(function(e){
			if(e.keyCode===13){newSearch();}
		});
		APIcall(
			//"mockapi/systemstatus_data.json",
			"/api/v0/status",
			"systemstatus",	$('#placeholder_systemstatus'));
		APIcall(
			//"mockapi/awaiting_approval.json",
			"/api/v0/awaitingApproval"+
			"?fields=hostname,reversedns,ipaddress,approvalId",
			"awaiting_approval", $('#placeholder_approval'));
		APIcall(
			//"mockapi/latestnewmachines.json",
			"/api/v0/hostlist?fields=hostname,certfp,lastseen"+
				"&rsort=lastseen&limit=10",
			"latestnewmachines", $('#placeholder_latestnewmachines'));
	});
}

function browseHostByCert(certfp) {
	APIcall(
		//"mockapi/browsehost.json",
		"/api/v0/host?certfp="+encodeURIComponent(certfp)+
		"&fields=ipAddress,hostname,lastseen,os,osEdition,"+
		"kernel,vendor,model,serialNo,clientVersion,certfp,files",
		"browsehost", "div#pageContent");
}

function browseFileById(fileId) {
	APIcall(
		//"mockapi/browsefile.json",
		"/api/v0/file?fields=fileId,lastModified,hostname,filename,"+
		"content,certfp,versions&fileId="+encodeURIComponent(fileId),
		"browsefile", "div#pageContent")
	.done(showDiff)
	.done(function(){
		$("select#selectVersion").val(fileId);
		$("select#selectVersion").change(function(){
			location.href = "#/browsefile/"+$(this).val();
		})
	});
}

function browseFileByName(hostname, filename) {
	filename = decodeURIComponent(filename);
	APIcall(
		//"mockapi/browsefile.json",
		"/api/v0/file?fields=fileId,lastModified,"+
		"hostname,filename,content,certfp,versions"+
		"&filename="+encodeURIComponent(filename)+
		"&hostname="+encodeURIComponent(hostname),
		"browsefile", "div#pageContent")
	.done(showDiff)
	.done(function(){
		$("select#selectVersion:first-child").prop("selected","selected");
		$("select#selectVersion").change(function(){
			location.href = "#/browsefile/"+$(this).val();
		})
	});
}

function newSearch() {
	let q = $("input#search").val();
	if (q == "") return;
	location.href = "#/search/" +
		encodeURIComponent(q);
}

function searchPage(q) {
	// show the spinner
	$("div#searchSpinner").fadeIn();
	$("div#searchResult").fadeOut();
	if (!q) q = "";
	else q = decodeURIComponent(q);
	APIcall(
		//"mockapi/searchpage.json",
		"/api/v0/searchpage?q="+encodeURIComponent(q)+
		"&page=1&hitsPerPage=10&excerpt=80",
		"searchpage", "div#pageContent")
	.done(function(){
		// add handlers to the input field and button
		$("button#searchButton").click(newSearch);
		$("input#search").keyup(function(e){
			if(e.keyCode===13){newSearch();}
		}).focus(); // focus the input field
	});
}
