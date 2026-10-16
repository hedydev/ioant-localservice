import {$,state,notice} from './core.js';
import {activateView} from './navigation.js';

const THEME_KEY='ils.theme';
const THEMES=new Set(['light','dark']);

function storedTheme(){
 try{
  const value=localStorage.getItem(THEME_KEY);
  return THEMES.has(value)?value:'light';
 }catch{
  return 'light';
 }
}

export function applyTheme(theme,{persist=true,notify=false}={}){
 const value=THEMES.has(theme)?theme:'light';
 document.documentElement.dataset.theme=value;
 document.documentElement.style.colorScheme=value;
 if(persist){
  try{localStorage.setItem(THEME_KEY,value);}catch{}
 }
 document.querySelectorAll('[data-theme-choice]').forEach(button=>{
  button.setAttribute('aria-pressed',String(button.dataset.themeChoice===value));
 });
 window.dispatchEvent(new CustomEvent('theme-changed',{detail:{theme:value}}));
 if(notify)notice(value==='dark'?'已切换为深色主题。':'已切换为浅色主题。','success');
 return value;
}

function ensureThemeStyle(){
 if(document.querySelector('link[data-ils-theme-style]'))return;
 const link=document.createElement('link');
 link.rel='stylesheet';
 link.href='/theme.css?v=20261014-contrast';
 link.dataset.ilsThemeStyle='1';
 document.head.appendChild(link);
}

function ensureSettingsNavigation(){
 if($('#settings-button'))return;
 const button=document.createElement('button');
 button.id='settings-button';
 button.type='button';
 button.className='quiet';
 button.setAttribute('aria-label','打开设置');
 button.textContent='设置';
 const adminButton=$('#admin-button');
 if(adminButton)adminButton.before(button);
 else document.querySelector('.header-actions')?.appendChild(button);
}

function ensureSettingsShell(){
 if($('#view-settings'))return;
 const section=document.createElement('section');
 section.id='view-settings';
 section.className='workspace-view settings-page';
 section.dataset.viewPanel='settings';
 section.hidden=true;
 section.innerHTML=`
   <div class="view-heading">
     <div><h2>设置</h2><p>管理当前浏览器的 ILS 界面偏好。服务级 Apple / OTA 配置使用左侧独立菜单。</p></div>
   </div>
   <div class="settings-grid">
     <article class="panel-card settings-card">
       <div class="section-heading">
         <div><h2>外观</h2><span class="meta">Theme</span></div>
         <span class="meta">立即生效</span>
       </div>
       <div class="theme-choice-grid" role="group" aria-label="主题">
         <button type="button" class="theme-choice" data-theme-choice="light" aria-pressed="false">
           <span class="theme-preview light" aria-hidden="true"></span>
           <span class="theme-choice-copy"><strong>浅色</strong><span>明亮背景，适合白天和光线充足环境。</span></span>
           <span class="theme-choice-check" aria-hidden="true">✓</span>
         </button>
         <button type="button" class="theme-choice" data-theme-choice="dark" aria-pressed="false">
           <span class="theme-preview dark" aria-hidden="true"></span>
           <span class="theme-choice-copy"><strong>深色</strong><span>降低夜间亮度，同时保留构建状态的可读性。</span></span>
           <span class="theme-choice-check" aria-hidden="true">✓</span>
         </button>
       </div>
       <p class="settings-local-note">主题只保存在当前浏览器，不影响 ILS 服务、项目配置或其他设备。</p>
     </article>
   </div>`;
 document.querySelector('main')?.insertBefore(section,document.querySelector('main > footer'));
}

function syncSettingsSelection(){
 applyTheme(document.documentElement.dataset.theme||storedTheme(),{persist:false});
 const active=state.view==='settings';
 $('#settings-button')?.classList.toggle('active',active);
 $('#settings-button')?.setAttribute('aria-pressed',String(active));
}

export function initSettings(){
 ensureThemeStyle();
 ensureSettingsNavigation();
 ensureSettingsShell();
 applyTheme(document.documentElement.dataset.theme||storedTheme(),{persist:false});

 $('#settings-button').onclick=()=>activateView('settings');
 $('#view-settings').addEventListener('click',event=>{
  const button=event.target.closest('[data-theme-choice]');
  if(!button)return;
  applyTheme(button.dataset.themeChoice,{notify:true});
 });

 window.addEventListener('view-changed',syncSettingsSelection);
 window.addEventListener('storage',event=>{
  if(event.key===THEME_KEY)applyTheme(THEMES.has(event.newValue)?event.newValue:'light',{persist:false});
 });
 syncSettingsSelection();

 if(state.view==='settings')activateView('settings',{updateRoute:false});
}
