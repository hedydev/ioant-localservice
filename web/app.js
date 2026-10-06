import {initAppStoreConnect} from './js/app-store-connect.js';
import {initPlatformUI} from './js/platform-ui.js';
import {initCore,restoreAdminSession} from './js/core.js';
import {initNavigation} from './js/navigation.js';
import {initProjects} from './js/projects.js';
import {initReleases} from './js/releases.js';
import {initBuildSource} from './js/build-source.js';
import {initBuildProfiles} from './js/build-profiles.js';
import {initBuildJobs} from './js/build-jobs.js';
import {initIOS} from './js/ios.js';
import {initServices} from './js/services.js';
import {initAutomation} from './js/automation.js';

const releaseCardStyles=document.createElement('link');
releaseCardStyles.rel='stylesheet';
releaseCardStyles.href='/release-cards.css';
document.head.append(releaseCardStyles);

initCore();
initPlatformUI();
initNavigation();
initReleases();
initBuildSource();
initBuildProfiles();
initBuildJobs();
initIOS();
initServices();
initAppStoreConnect();
initAutomation();

await restoreAdminSession();
initProjects();
