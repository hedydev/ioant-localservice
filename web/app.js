import {initCore,restoreAdminSession,notice} from './js/core.js';
import {initProjects,refreshData} from './js/projects.js';

const releaseCardStyles=document.createElement('link');
releaseCardStyles.rel='stylesheet';
releaseCardStyles.href='/release-cards.css';
document.head.append(releaseCardStyles);

const optionalModules=[
 ['./js/platform-ui.js','initPlatformUI','平台 UI'],
 ['./js/navigation.js','initNavigation','导航'],
 ['./js/releases.js','initReleases','Release'],
 ['./js/build-source.js','initBuildSource','构建来源'],
 ['./js/build-profiles.js','initBuildProfiles','Release Profile'],
 ['./js/build-jobs.js','initBuildJobs','构建任务'],
 ['./js/ios.js','initIOS','iOS'],
 ['./js/services.js','initServices','设备与 OTA'],
 ['./js/app-store-connect.js','initAppStoreConnect','App Store Connect'],
 ['./js/automation.js','initAutomation','自动化']
];

async function initOptionalModule([path,exportName,label]){
 try{
  const module=await import(path);
  const initializer=module[exportName];
  if(typeof initializer!=='function')throw new Error('缺少初始化入口 '+exportName);
  initializer();
  return {ok:true,label};
 }catch(error){
  console.error('ILS optional module failed:',label,error);
  return {ok:false,label,error};
 }
}

async function finishBootstrap(){
 const results=await Promise.all(optionalModules.map(initOptionalModule));
 const failed=results.filter(result=>!result.ok);
 if(failed.length){
  const labels=failed.map(result=>result.label).join('、');
  notice('部分功能模块初始化失败：'+labels+'。项目列表和基础功能仍可使用；请查看浏览器 Console。','error',0);
 }

 // Admin-only modules are now listening before the remembered session is
 // restored. This avoids losing admin-loaded during startup.
 await restoreAdminSession();

 // Re-broadcast current project/release state after optional listeners exist.
 // This also repairs a race where the first public project fetch completes
 // before a non-core module finished loading.
 try{
  await refreshData();
 }catch(error){
  console.warn('ILS post-bootstrap refresh deferred:',error);
 }
}

// The project list is the core shell. Start it before any optional module so a
// broken Build/OTA/TestFlight UI can never leave the whole application stuck on
// “正在连接”. Optional modules are dynamically imported so even parse/link errors
// in one of them cannot prevent this core bootstrap from running.
initCore();
initProjects();
void finishBootstrap();
