import openApiDocument from "../../../../backend/openapi.yaml?raw";
import type { RequestHandler } from "./$types";

export const GET: RequestHandler = () => {
  return new Response(openApiDocument, {
    headers: {
      "Cache-Control": "no-cache",
      "Content-Type": "application/yaml; charset=utf-8",
    },
  });
};
