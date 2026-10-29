const $ = id => document.getElementById(id);
const escape = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const safeLink = s => {try {const u=new URL(s);return u.protocol==='https:' ? escape(u.href) : '#';}catch{return '#';}};
const date = s => s && !Number.isNaN(Date.parse(s)) ? new Date(s).toLocaleDateString('en-GB',{day:'numeric',month:'short',year:'numeric',timeZone:'UTC'}) : 'Not provided';
const statusNames = {checked:'Checked',partial:'Partial scan',failed:'Check failed',manual_check:'Manual check',needs_attention:'Needs attention',needs_verification:'Verify link',not_configured:'No link yet',not_run:'Not run yet'};
const jobStatus = {listed:'Listed on latest check',closed:'Past deadline',not_listed:'No longer listed',unverified:'Needs rechecking'};
let report = {jobs:[],checks:[],keywords:[]};let page=1;let filtered=[];const pageSize=15;

function renderJobs(){
 const query=$('search').value.trim().toLowerCase(), council=$('council-filter').value,status=$('status-filter').value;
 filtered=report.jobs.filter(j=> (!query||[j.title,j.council,j.description,j.location].some(v=>String(v||'').toLowerCase().includes(query)))&&(!council||j.council===council)&&(status==='all'||(status==='current'?['listed','unverified'].includes(j.status):status===j.status)));
 const sort=$('sort').value;filtered.sort((a,b)=>sort==='deadline'?(a.deadline||'9999').localeCompare(b.deadline||'9999'):sort==='council'?a.council.localeCompare(b.council):(b.first_seen||'').localeCompare(a.first_seen||''));
 const pages=Math.max(1,Math.ceil(filtered.length/pageSize));page=Math.min(page,pages);
 $('results').textContent=`${filtered.length} matching ${filtered.length===1?'vacancy':'vacancies'} · ${(report.keywords||[]).join(' / ')}`;
 $('jobs').innerHTML=filtered.slice((page-1)*pageSize,page*pageSize).map(j=>`<article class="job"><div><div class="job-top"><span class="council-name">${escape(j.council)}</span><span class="badge ${j.status==='listed'?'':j.status==='unverified'?'warning':'neutral'}">${escape(jobStatus[j.status]||j.status)}</span></div><h2>${escape(j.title)}</h2><div class="job-meta"><span>${escape(j.location||'Location not provided')}</span><span>${escape(j.salary||'Salary not provided')}</span></div><p class="description">${escape((j.description||'').slice(0,260))}${(j.description||'').length>260?'…':''}</p><div class="job-meta"><span>Sponsorship: ${escape(j.sponsorship||'Not stated — verify with employer')}</span></div><div class="job-tags">${(j.matched||[]).map(k=>`<span class="chip">${escape(k)}</span>`).join('')}</div></div><div class="job-side"><span class="deadline">${escape(date(j.deadline))}</span><span class="label">APPLICATION DEADLINE</span><a class="apply" href="${safeLink(j.url)}" target="_blank" rel="noopener noreferrer">Read advert ↗</a><div class="dates">Posted: ${escape(date(j.posted))}<br>First found: ${escape(date(j.first_seen))}<br>Last seen: ${escape(date(j.last_seen))}</div></div></article>`).join('');
 $('jobs-empty').hidden=filtered.length>0;
 const h=$('jobs-empty').querySelector('h2'),p=$('jobs-empty').querySelector('p');
 h.textContent=report.jobs.length?'No vacancies match these filters':report.generated_at?'No matching vacancies were recorded':'Awaiting the first check';
 p.textContent=report.jobs.length?'Try another council, a broader search, or All statuses.':'Check Council coverage for scan failures, unsupported sites and links still to be added. An empty report does not confirm there are no relevant vacancies.';
 $('previous').disabled=page<=1;$('next').disabled=page>=pages;$('page-label').textContent=`${page} / ${pages}`;
}
function renderCoverage(){const query=$('coverage-search').value.toLowerCase(),linked=$('linked-only').checked;const rows=report.checks.filter(c=>(!linked||c.url)&&(!query||c.name.toLowerCase().includes(query)));$('coverage').innerHTML=rows.map(c=>`<tr><td>${escape(c.name)}</td><td><span class="badge ${c.status==='checked'?'':['not_configured','not_run'].includes(c.status)?'neutral':'warning'}">${escape(statusNames[c.status]||c.status)}</span></td><td>${escape(c.message)}<br><span class="small">${c.checked_at?`Checked ${escape(date(c.checked_at))} · ${c.reviewed||0} descriptions reviewed`:'No completed check recorded'}</span></td><td>${c.url?`<a href="${safeLink(c.url)}" target="_blank" rel="noopener noreferrer">Open ↗</a>`:'—'}</td></tr>`).join('')||'<tr><td colspan="4">No councils match this filter.</td></tr>';}
for(const id of ['search','council-filter','status-filter','sort'])$(id).addEventListener(id==='search'?'input':'change',()=>{page=1;renderJobs();});
$('previous').addEventListener('click',()=>{page--;renderJobs();});$('next').addEventListener('click',()=>{page++;renderJobs();});
for(const id of ['coverage-search','linked-only'])$(id).addEventListener(id==='coverage-search'?'input':'change',renderCoverage);
for(const tab of ['jobs','councils'])$(`${tab}-tab`).addEventListener('click',()=>{for(const name of ['jobs','councils']){$(`${name}-tab`).setAttribute('aria-selected',String(name===tab));$(`${name}-panel`).hidden=name!==tab;}});
$('export').addEventListener('click',()=>{
 const headers=['title','council','location','salary','posted','deadline','first_seen','last_seen','status','sponsorship','url'];
 const cell=s=>{let v=String(s??'');if(/^[\s]*[=+@-]/.test(v))v="'"+v;return '"'+v.replace(/"/g,'""')+'"';};
 const csv='\ufeff'+[headers,...filtered.map(j=>headers.map(k=>j[k]))].map(r=>r.map(cell).join(',')).join('\r\n');const u=URL.createObjectURL(new Blob([csv],{type:'text/csv;charset=utf-8'}));const a=document.createElement('a');a.href=u;a.download='council-job-matches.csv';a.click();setTimeout(()=>URL.revokeObjectURL(u),1000);
});
async function init(){try{const res=await fetch('./jobs.json',{cache:'no-store'});if(!res.ok)throw new Error('Report unavailable');report=await res.json();if(!Array.isArray(report.jobs)||!Array.isArray(report.checks))throw new Error('Invalid report');
 $('keywords').innerHTML=report.keywords.map(k=>`<span class="chip">${escape(k)}</span>`).join('');
 $('count-jobs').textContent=report.jobs.filter(j=>j.status==='listed').length;$('count-checked').textContent=report.checks.filter(c=>c.status==='checked').length;$('count-linked').textContent=report.checks.filter(c=>c.url).length;$('count-attention').textContent=report.checks.filter(c=>c.url&&!['checked'].includes(c.status)).length;
 if(report.generated_at){$('last-run').textContent=date(report.generated_at);$('run-note').textContent=`Report generated ${new Date(report.generated_at).toLocaleTimeString('en-GB',{hour:'2-digit',minute:'2-digit',timeZone:'UTC'})} UTC. See council coverage for individual check results.`;}else{$('run-note').textContent='Initial setup. This is not a live vacancy report yet.';}
 for(const name of [...new Set(report.jobs.map(j=>j.council))].sort()){const o=document.createElement('option');o.value=name;o.textContent=name;$('council-filter').append(o);}renderJobs();renderCoverage();
 }catch(e){$('results').textContent='The job report could not be loaded. Please retry later.';$('jobs-empty').hidden=false;$('jobs-empty').querySelector('h2').textContent='Report unavailable';$('jobs-empty').querySelector('p').textContent='The website needs jobs.json from the scheduled checker. If previewing locally, use an HTTP server instead of opening index.html directly.';}}
init();
