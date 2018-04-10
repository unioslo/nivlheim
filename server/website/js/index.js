$(document).ready(function(){
	Handlebars.registerHelper('formatDateTime', function(s){
		if (!s) return "";
		let t = moment(s);
		let str = t.fromNow() + ' (' + t.format('D MMM Y HH:mm') + ')';
		str = Handlebars.Utils.escapeExpression(str);
		if (t.isAfter(moment().subtract(1,'days'))) {
			return str;
		} else {
			return new Handlebars.SafeString(
				'<span class="underline-warning">'+str+'</span>');
		}
	});
	Handlebars.registerHelper('formatInterval', function(seconds){
		if (!seconds || seconds <= 0) return "0";
		let epoch = Math.floor((new Date).getTime()/1000);
		let m = moment.unix(epoch-seconds);
		let str = m.fromNow(true);
		return Handlebars.Utils.escapeExpression(str);
	});
	Handlebars.registerHelper('urlescape', function(s){
		if (!s) return "";
		return encodeURIComponent(s);
	});
	Handlebars.registerHelper('ifmatch', function(a,b,options){
		if (a.match(b)) return options.fn(this);
		return options.inverse(this);
	});
	Handlebars.registerHelper('ifcmp', function(a,operator,b,options){
		switch (operator) {
			case '=': if (a == b) return options.fn(this); break;
			case '!=': if (a != b) return options.fn(this); break;
			case '<': if (a < b) return options.fn(this); break;
			case '>': if (a > b) return options.fn(this); break;
		}
		return options.inverse(this);
	});
	Handlebars.registerHelper('pagination', function(page, maxPage, block){
		let accum = '';
		if (page-5>1) {
			accum += block.fn(1);
		}
		if (page-5>2) {
			accum += block.fn('...');
		}
		let from = Math.max(page-5, 1);
		let to = Math.min(Math.max(page+5, 10), maxPage);
		for(let i = from; i <= to; ++i)
			accum += block.fn(i);
		if (to < maxPage-1) {
			accum += block.fn('...');
		}
		if (to < maxPage) {
			accum += block.fn(maxPage);
		}
		return accum;
	});
	Handlebars.registerHelper('previous', function(n,block){
		if (n > 1) { return block.fn(n-1); }
		return "";
	});
	Handlebars.registerHelper('next', function(n,m,block){
		if (n<m) { return block.fn(n+1); }
		return "";
	});

	var routes = {
		'/allhosts': allHosts,
		'/browsehost/:certfp': browseHostByCert,
		'/browsefile/:fileId': browseFileById,
		'/browsefile/:hostname/:filename': browseFileByName,
		'/search/:page/:query': searchPage,
		'/search': searchPage,
		'/settings': settingsPage,
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
	router.param('page', /(\d+)/);

	router.init('/');

	$.get('/version.txt', function(data){
		$("span#navbarVersion").text('Version ' + data);
	});

	// handle the "burger" menu icon that appears on narrow screens
	$("div.navbar-burger").click(function(){
		$(this).toggleClass('is-active');
		$("div.navbar-menu").toggleClass('is-active');
	});
	$("a.navbar-item").click(function(){
		$("div.navbar-burger").removeClass('is-active');
		$("div.navbar-menu").removeClass('is-active');
	});

	// load more elements when scrolling
	$(window).scroll(scrollHandler);
	window.setInterval(scrollHandler,500);
});

function showFrontPage() {
	renderTemplate("frontpage", {}, "div#pageContent")
	.done(function(){
		$("button#searchButton").click(newSearch);
		$("input#search").keyup(function(e){
			if(e.keyCode===13){newSearch();}
		});
		autoReloadStatus();
		APIcall(
			//"mockapi/awaiting_approval.json",
			"/api/v0/awaitingApproval"+
			"?fields=hostname,reversedns,ipaddress,approvalId",
			"awaiting_approval", $('#placeholder_approval'));
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
	location.href = "#/search/1/" +
		encodeURIComponent(q);
}

function searchPage(page, q) {
	if (!q) q = "";
	else q = decodeURIComponent(q);
	//if ($("div#searchSpinner").length == 0) {
	// if we're not already on the search page, render the template
	renderTemplate("searchpage", {"query": q}, "div#pageContent")
	.done(function(){
		// add handlers to the input field and button
		$("button#searchButton").click(newSearch);
		$("input#search").keyup(function(e){
			if(e.keyCode===13){newSearch();}
		}).focus(); // focus the input field
		// no query? then exit
		if (q == "") return;
		// show the spinner
		$("div#searchSpinner").fadeIn();
		// search host names
		APIcall("/api/v0/hostlist?fields=hostname,certfp&hostname="+
			encodeURIComponent("*"+q.replace(' ','*')+"*"), "searchresulthostnames",
			"div#searchResultHostnames");
		// search files
		APIcall(
			//"mockapi/searchpage.json",
			"/api/v0/searchpage?q="+encodeURIComponent(q)+
			"&page="+page+"&hitsPerPage=8",
			"searchresultfiles", "div#searchResult")
		.done(function(){
			// hide the spinner
			$("div#searchSpinner").hide();
		});
	});
/*
	if ($("div#searchSpinner").length == 0) {
		// if we're not already on the search page, show a blank page
		$("div#pageContent").children().fadeOut().remove();
		$("div#pageContent").append('<section class="section">'+
			'<div class="container"><span class="icon">'+
			'<i class="fas fa-cog fa-3x fa-spin"></i></span></div></section>');
	} else {
		// show the spinner
		$("div#searchSpinner").fadeIn();
		$("div#searchResult").fadeOut();
	}*/
}

function allHosts() {
	// are we already on the browse page? then don't reload it
	if ($("aside.menu").length>0) return;
	// retrieve lists of OSes, Manufacturers, etc.
	let pfx = getAPIURLprefix();
	let promises = [];
	promises.push($.get(pfx+"/api/v0/hostlist?group=os"));
	promises.push($.get(pfx+"/api/v0/hostlist?group=osEdition"));
	promises.push($.get(pfx+"/api/v0/hostlist?group=vendor"));
	promises.push($.get(pfx+"/api/v0/hostlist?group=model"));
	// wait for all the promises to complete
	$.when.apply($, promises).then(function(){
		// remove entries that are the string "null"
		for (let i=0; i<arguments.length; i++)
			delete arguments[i][0]["null"];
		// compose an object to send to the handlebars template
		var data = {
			"os": arguments[0][0],
			"osEdition": arguments[1][0],
			"manufacturer": arguments[2][0],
			"product": arguments[3][0],
		};
		renderTemplate("allhosts", data, "div#pageContent")
		.done(function(){
			// Look at url parameters (if any) and select appropriate items
			let p = getUrlParams();
			if (p["q"]) {
				let a = p["q"].split(',');
				for (let i=0; i<a.length; i++) {
					$("aside.menu li a:contains('"+a[i]+"')").addClass('is-active');
				}
			}
			// Load matching hosts
			reloadMatchingHosts();
			// Add event handlers
			$("aside.menu li a").click(function(){
				// toggle this item on/off
				$(this).toggleClass('is-active');
				reloadMatchingHosts();
			});
		});
	});
}

function reloadMatchingHosts() {
	// find the selected elements from the menu
	let oses = [];
	$("aside.menu li a.os.is-active span:first-of-type").each(function(i,e){
		oses.push(e.innerText);
	});
	let editions = [];
	$("aside.menu li a.osEdition.is-active span:first-of-type").each(function(i,e){
		editions.push(e.innerText);
	});
	let manufacturers = [];
	$("aside.menu li a.manufacturer.is-active span:first-of-type").each(function(i,e){
		manufacturers.push(e.innerText);
	});
	let products = [];
	$("aside.menu li a.product.is-active span:first-of-type").each(function(i,e){
		products.push(e.innerText);
	});
	// set the query string in the url
	let q = oses.concat(editions).concat(manufacturers).concat(products).join(',');
	if (q) q = "?q="+q;
	location.assign("/#/allhosts"+q);
	// prepare the API call that loads the list of hosts that match
	q = "/api/v0/hostlist?fields=hostname,certfp";
	if (oses.length>0) q += "&os="+oses.join(',');
	if (editions.length>0) q += "&osEdition="+editions.join(',');
	if (manufacturers.length>0) q += "&vendor="+manufacturers.join(',');
	if (products.length>0) q += "&model="+products.join(',');
	$("div#hostlist").data("query",q).data("offset",0).html("");
	loadMoreHosts();
}

function loadMoreHosts() {
	let q = $("div#hostlist").data("query");
	let offset = $("div#hostlist").data("offset") || 0;
	//console.log("Loading from "+offset);
	let limit = 30;
	APIcall(q + "&limit="+limit+"&offset="+offset, "hostlist",
		"div#morehosts")
	.done(function(){
		$("div#hostlist").data("offset", offset + limit);
		$("div#morehosts").children().appendTo("div#hostlist");
		$("div.loadmore").scroll(loadMoreHosts);
	});
}

function settingsPage() {
	renderTemplate("settingspage", {}, "div#pageContent")
	.done(function(){
		APIcall(
			//"mockapi/ipranges.json",
			"/api/v0/settings/ipranges?fields=ipRangeId,ipRange,comment,useDns",
			"ipranges", "div#ipranges_placeholder")
		.done(function(){
			attachHandlersToForms();
		});
	});
}
