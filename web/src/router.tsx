import { createBrowserRouter, Navigate } from 'react-router-dom'
import { App } from './App'
import { Channels }     from './pages/Channels'
import { Vods }         from './pages/Vods'
import { M3UEPGManager } from './pages/M3UEPGManager'
import { TvGuide }      from './pages/TvGuide'
import { Dvr }          from './pages/Dvr'
import { Stats }        from './pages/Stats'
import { Plugins }      from './pages/Plugins'
import { Users }        from './pages/Users'
import { LogoManager }  from './pages/LogoManager'
import { Settings }     from './pages/Settings'

export const router = createBrowserRouter([
  {
    path: '/',
    element: <App />,
    children: [
      { index: true, element: <Navigate to="/channels" replace /> },
      { path: 'channels',  element: <Channels />     },
      { path: 'vods',      element: <Vods />          },
      { path: 'm3u-epg',  element: <M3UEPGManager /> },
      { path: 'tv-guide', element: <TvGuide />        },
      { path: 'dvr',      element: <Dvr />            },
      { path: 'stats',    element: <Stats />          },
      { path: 'plugins',  element: <Plugins />        },
      { path: 'users',    element: <Users />          },
      { path: 'logos',    element: <LogoManager />    },
      { path: 'settings', element: <Settings />       },
    ],
  },
])
