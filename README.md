# esp-updater

arduino code:

```c++

WiFiClient espClient;

#define AUTO_WIFI_UPDATES (true)

#if AUTO_WIFI_UPDATES
#include <ESP8266httpUpdate.h>

boolean httpUpdateInProgress = false;
#endif

void setup() {

  // your setup stuff comes here..
  // ...

#if AUTO_WIFI_UPDATES

   ESPhttpUpdate.onProgress([](int cur, int total) {
      static int last = 0;
      if (cur == 0 || cur == total || cur - last > 1000) {
         Serial.printf("%d/%d\n", cur, total);
         last = cur;
      }
   });

   ESPhttpUpdate.onStart([]() {
      Serial.printf("update starts\n");
      httpUpdateInProgress = true;
   });

   ESPhttpUpdate.onEnd([]() {
      Serial.printf("update ends\n");
      httpUpdateInProgress = false;
   });

   t_httpUpdate_return updateResult = ESPhttpUpdate.update(
       espClient,
       "your-update-server-name-comes-here", // hostname or ip
       35982,
       "/update/your-application-name", // .. update server will search in this folder then!
       "1.2.3"); // needs to be a semver version string here!!

   switch (updateResult) {
      case HTTP_UPDATE_FAILED:
         Serial.println("[update] Update failed.");
         break;

      case HTTP_UPDATE_NO_UPDATES:
         Serial.println("[update] Update no Update.");
         break;

      case HTTP_UPDATE_OK:
         Serial.println("[update] Update ok.");  // may not be called since we reboot the ESP
         break;
   }

#endif

}
```
