import {$,state} from './core.js';
import {activateView} from './navigation.js';

function ensureAppleNavigation(){
 const nav=$('#global-services-nav');
 if(!nav||$('#global-apple-button'))return;
 const button=document.createElement('button');
 button.id='global-apple-button';
 button.type='button';
 button.dataset.globalView='apple';
 button.innerHTML='Apple Developer<small>App Store Connect / Signing</small>';
 nav.appendChild(button);
}

function ensureAppleShell(){
 if($('#view-apple'))return;
 const section=document.createElement('section');
 section.id='view-apple';
 section.className='workspace-view settings-page';
 section.dataset.viewPanel='apple';
 section.hidden=true;
 section.innerHTML=`
  <div class="view-heading">
    <div><h2>Apple Developer</h2><p>ILS 服务级 Apple 配置，所有项目共享；项目只保留自己的 Release Profile 和发布记录。</p></div>
  </div>
  <div id="apple-global-grid" class="settings-grid"></div>`;
 document.querySelector('main')?.insertBefore(section,document.querySelector('main > footer'));
}

function relocateGlobalConfiguration(){
 const grid=$('#apple-global-grid');
 const status=$('#asc-status');
 const card=status?.closest('article.panel-card');
 if(!grid||!card)return;
 card.classList.add('settings-card','global-settings-card');
 card.dataset.globalSettingsCard='app-store-connect';
 const heading=card.querySelector('.section-heading h2');
 const headingMeta=card.querySelector('.section-heading .meta');
 const description=card.querySelector(':scope > p');
 if(heading)heading.textContent='App Store Connect / Apple Developer';
 if(headingMeta)headingMeta.textContent='全局 · TestFlight / 设备注册';
 if(description)description.textContent='API Key、Apple Team 绑定与 TestFlight 状态同步属于整个 ILS；Sowhat、IWB、ChatDock、NetScale 等项目共用同一套 Apple 服务配置。';
 grid.appendChild(card);
}

function syncSelection(){
 const active=state.view==='apple';
 $('#global-apple-button')?.classList.toggle('active',active);
 if(active)$('#global-apple-button')?.setAttribute('aria-current','page');
 else $('#global-apple-button')?.removeAttribute('aria-current');
}

export function initAppleGlobal(){
 ensureAppleNavigation();
 ensureAppleShell();
 relocateGlobalConfiguration();
 syncSelection();

 window.addEventListener('view-changed',()=>{
  relocateGlobalConfiguration();
  syncSelection();
 });

 if(state.view==='apple')activateView('apple',{updateRoute:false});
}
